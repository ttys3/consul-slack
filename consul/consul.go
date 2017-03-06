package consul

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	lockKey  = "consul-slack-lock"
	stateKey = "consul-slack-state"
)

// New creates new consul client
func New(cfg *Config) (*Consul, error) {
	if cfg == nil {
		panic("cfg is nil")
	}

	c, err := api.NewClient(&api.Config{
		Address:    cfg.Address,
		Scheme:     cfg.Scheme,
		Datacenter: cfg.Datacenter,
	})

	if err != nil {
		return nil, err
	}

	cc := &Consul{
		kvAPI:      c.KV(),
		healthAPI:  c.Health(),
		sessionAPI: c.Session(),

		interval: cfg.Interval,
		stopCh:   make(chan struct{}),
	}

	if err = cc.startSession(); err != nil {
		return nil, err
	}

	return cc, nil
}

// Config is consul configuration
type Config struct {
	Address    string
	Scheme     string
	Datacenter string
	Interval   time.Duration
}

// Consul is the consul server client
type Consul struct {
	kvAPI      *api.KV
	healthAPI  *api.Health
	sessionAPI *api.Session

	stopCh   chan struct{}
	interval time.Duration
	cc       api.HealthChecks
}

// startSession creates new consul session and holds an unique lock
func (c *Consul) startSession() error {
	sess, _, err := c.sessionAPI.Create(&api.SessionEntry{
		Behavior: "delete",
		TTL:      "30s",
	}, nil)

	if err != nil {
		return err
	}

	c.infof("%s created", sess)
	c.infof("%s lock", sess)

	// lock
	lock := &api.KVPair{
		Key:   lockKey,
		Value: []byte{'o', 'k'},
		Session: sess,
	}

	for {
		ok, _, err := c.kvAPI.Acquire(lock, nil)
		if err != nil {
			return err
		}

		if ok {
			c.infof("%s acquired", sess)
			break
		}

		// wait before next iteration
		time.Sleep(time.Second)
	}

	// renew session in the background
	go func() {
	Loop:
		for {
			select {
			case <-c.stopCh:
				// unlock
				c.infof("%s release", sess)
				_, _, err := c.kvAPI.Release(lock, nil)
				if err != nil {
					c.infof("release lock error: %v", err)
				}

				// destroy
				c.infof("%s destroy", sess)
				_, err = c.sessionAPI.Destroy(sess, nil)
				if err != nil {
					c.infof("destroy session error: %v", err)
				}

				break Loop
			case <-time.After(15 * time.Second):
				_, _, err := c.sessionAPI.Renew(sess, nil)
				if err != nil {
					c.infof("renew session error: %v", err)
					return
				}
			}
		}
	}()

	return nil
}

// Close shuts down Next() function
func (c *Consul) Close() error {
	close(c.stopCh)
	return nil
}

// load loads consul state from kv store
func (c *Consul) load() (api.HealthChecks, error) {
	kv, _, err := c.kvAPI.Get(stateKey, nil)
	if err != nil {
		return nil, err
	}

	chs := api.HealthChecks{}

	if kv != nil {
		err = json.Unmarshal(kv.Value, &chs)
	}

	return chs, err
}

// dump saves consul state to kv store
func (c *Consul) dump(chs api.HealthChecks) error {
	b, err := json.Marshal(chs)
	if err != nil {
		return err
	}

	_, err = c.kvAPI.Put(&api.KVPair{
		Key:   stateKey,
		Value: b,
	}, nil)

	return err
}

// Next returns slices of critical and passing events
func (c *Consul) Next() (cc api.HealthChecks, pc api.HealthChecks, err error) {
	var hc api.HealthChecks

	// start immediately
	t := time.NewTimer(time.Duration(0))

	if c.cc == nil {
		c.cc, err = c.load()
		if err != nil {
			return
		}

		c.infof("initial state is %v", c.cc)
	}

	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			hc, _, err = c.healthAPI.State("critical", nil)
			if err != nil {
				return
			}

			// passing
			for _, check := range c.cc {
				if pos(hc, check) != -1 {
					continue
				}

				pc = append(pc, check)
				c.cc = del(c.cc, check)
				c.infof("[%s] %s is passing", check.Node, check.ServiceName)
			}

			// critical
			for _, check := range hc {
				if pos(c.cc, check) != -1 {
					continue
				}

				cc = append(cc, check)
				c.cc = append(c.cc, check)
				c.infof("[%s] %s is failing", check.Node, check.ServiceName)
			}

			// save state
			if err = c.dump(c.cc); err != nil {
				return
			}

			if len(cc) > 0 || len(pc) > 0 {
				return
			}

			t = time.NewTimer(c.interval)
		}
	}
}

// infof prints a debug message to stderr when debug mode is enabled
func (c *Consul) infof(s string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, "consul: "+s+"\n", v...)
}

// pos finds check's position in health checks slice or return -1
func pos(hcs api.HealthChecks, hc *api.HealthCheck) int {
	for i, c := range hcs {
		if c.ServiceID == hc.ServiceID {
			return i
		}
	}
	return -1
}

// del deletes the named element from health checks slice
func del(hcs api.HealthChecks, hc *api.HealthCheck) api.HealthChecks {
	i := pos(hcs, hc)
	if i == -1 {
		return hcs
	}
	return append(hcs[:i], hcs[i+1:]...)
}
