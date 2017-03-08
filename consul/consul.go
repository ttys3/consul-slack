package consul

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	lockKey  = "consul-slack/.lock"
	stateKey = "consul-slack/state"
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

	// check agent connection
	_, err = c.Status().Leader()
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	cc := &Consul{
		kvAPI:      c.KV(),
		healthAPI:  c.Health(),
		sessionAPI: c.Session(),

		stopCh: make(chan struct{}),
		nextCh: make(chan *event),
	}

	if err = cc.startSession(); err != nil {
		return nil, err
	}

	go cc.watch()

	return cc, nil
}

// Config is consul configuration
type Config struct {
	Address    string
	Scheme     string
	Datacenter string
}

// Consul is the consul server client
type Consul struct {
	kvAPI      *api.KV
	healthAPI  *api.Health
	sessionAPI *api.Session

	lock   *api.KVPair
	locked bool
	stopCh chan struct{}
	nextCh chan *event
}

var (
	ttl      = "30s"
	waitTime = 15 * time.Second
)

// startSession creates new consul session and holds an unique lock
func (c *Consul) startSession() error {
	sess, _, err := c.sessionAPI.Create(&api.SessionEntry{
		Behavior: "delete",
		TTL:      ttl,
	}, nil)

	if err != nil {
		return err
	}

	c.infof("%s created", sess)

	go func() {
		if err = c.sessionAPI.RenewPeriodic(ttl, sess, nil, c.stopCh); err != nil {
			c.infof("% renew error %v", err)
		}
	}()

	// lock
	c.infof("%s lock", sess)
	c.lock = &api.KVPair{
		Key:     lockKey,
		Value:   []byte{'o', 'k'},
		Session: sess,
	}

	var waitIndex uint64

	for {
		kv, _, err := c.kvAPI.Get(lockKey, &api.QueryOptions{
			WaitTime:  waitTime,
			WaitIndex: waitIndex,
		})

		if err != nil {
			return err
		}

		if kv != nil {
			waitIndex = kv.ModifyIndex
		}

		ok, _, err := c.kvAPI.Acquire(c.lock, nil)
		if err != nil {
			return err
		}

		if ok {
			c.infof("%s acquired", sess)
			c.locked = true
			break
		}
	}

	return nil
}

// Close shuts down Next() function
func (c *Consul) Close() error {
	if c.locked {
		c.infof("%s release", c.lock.Session)
		_, _, err := c.kvAPI.Release(c.lock, nil)
		if err != nil {
			return err
		}
	}

	// destroy session
	c.infof("%s destroy", c.lock.Session)
	_, err := c.sessionAPI.Destroy(c.lock.Session, nil)
	if err != nil {
		return err
	}

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
func (c *Consul) Next() (api.HealthChecks, api.HealthChecks, error) {
	for {
		select {
		case ev := <-c.nextCh:
			return ev.cc, ev.pc, ev.err
		case <-c.stopCh:
			return nil, nil, nil
		}
	}
}

type event struct {
	cc  api.HealthChecks
	pc  api.HealthChecks
	err error
}

func (c *Consul) watch() {
	// load state
	sc, err := c.load()
	if err != nil {
		c.infof("load state error %v", err)
	}
	c.infof("initial state is %v", sc)

	hc := api.HealthChecks{}
	meta := &api.QueryMeta{}

	for {
		select {
		case <-c.stopCh:
			goto End
		default:
		}

		pc := api.HealthChecks{}
		cc := api.HealthChecks{}
		hc, meta, err = c.healthAPI.State("critical", &api.QueryOptions{
			AllowStale: true,
			WaitIndex:  meta.LastIndex,
			WaitTime:   waitTime,
		})

		if err != nil {
			c.nextCh <- &event{err: err}
			goto End
		}

		// passing
		for _, check := range sc {
			if pos(hc, check) != -1 {
				continue
			}

			pc = append(pc, check)
			sc = del(sc, check)
			c.infof("[%s] %s is passing", check.Node, check.ServiceName)
		}

		// critical
		for _, check := range hc {
			if pos(sc, check) != -1 {
				continue
			}

			cc = append(cc, check)
			sc = append(sc, check)
			c.infof("[%s] %s is failing", check.Node, check.ServiceName)
		}

		// save state
		if err = c.dump(sc); err != nil {
			c.nextCh <- &event{err: err}
			goto End
		}

		if len(cc) > 0 || len(pc) > 0 {
			c.nextCh <- &event{cc: cc, pc: pc}
		}
	}

End:
	close(c.nextCh)
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
