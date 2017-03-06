package consul

import (
	"fmt"
	"os"

	"time"

	"github.com/hashicorp/consul/api"
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

	sessionID, _, err := c.Session().Create(&api.SessionEntry{
		Name:     "consul-slack-lock",
		Behavior: "delete",
		TTL:      "15s",
	}, nil)

	if err != nil {
		return nil, err
	}

	return &Consul{
		kvAPI:      c.KV(),
		healthAPI:  c.Health(),
		sessionAPI: c.Session(),

		lock: &api.KVPair{
			Key:     "consul-slack-lock",
			Value:   []byte{'o', 'k'},
			Session: sessionID,
		},

		interval: cfg.Interval,
	}, nil
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

	lock     *api.KVPair
	lockCh   chan struct{}
	interval time.Duration

	// TODO: use KV
	// cc is critical checks
	cc api.HealthChecks
}

// Lock blocks until lock is acquired
func (c *Consul) Lock() error {
	if c.lockCh != nil {
		panic("already locked")
	}

	c.infof("%s lock", c.lock.Session)

	for {
		ok, _, err := c.kvAPI.Acquire(c.lock, nil)
		if err != nil {
			return err
		}

		if ok {
			c.infof("%s acquired", c.lock.Session)
			break
		}

		// wait before next iteration
		time.Sleep(time.Second)
	}

	c.lockCh = make(chan struct{})

	// renew session in background
	go func() {
	Loop:
		for {
			select {
			case <-c.lockCh:
				break Loop
			case <-time.After(10 * time.Second):
				_, _, err := c.sessionAPI.Renew(c.lock.Session, nil)
				if err != nil {
					return
				}
			}
		}
	}()

	return nil
}

// Unlock releases previously created lock
func (c *Consul) Unlock() error {
	if c.lockCh == nil {
		panic("not locked")
	}

	close(c.lockCh)
	c.infof("%s release", c.lock.Session)
	_, _, err := c.kvAPI.Release(c.lock, nil)
	return err
}

// Next returns slices of critical and passing events
func (c *Consul) Next() (cc api.HealthChecks, pc api.HealthChecks, err error) {
	var hc api.HealthChecks

	for {
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

		if len(cc) > 0 || len(pc) > 0 {
			return
		}

		time.Sleep(c.interval)
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
