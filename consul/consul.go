package consul

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	lockKey  = "consul-slack/.lock"
	stateKey = "consul-slack/state"
)

var ErrClosed = errors.New("consul: closed")

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
		api:    c,
		stopCh: make(chan struct{}),
		nextCh: make(chan *payload),
	}

	if err = cc.createSession(); err != nil {
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
	api    *api.Client
	lock   *api.KVPair
	locked bool
	stopCh chan struct{}
	nextCh chan *payload
}

var (
	ttl      = "30s"
	waitTime = 15 * time.Second
)

// createSession creates new consul session and holds an unique lock
func (c *Consul) createSession() error {
	sess, _, err := c.api.Session().Create(&api.SessionEntry{
		Behavior:  "delete",
		TTL:       ttl,
		LockDelay: time.Second,
	}, nil)

	if err != nil {
		return err
	}

	c.lock = &api.KVPair{
		Key:     lockKey,
		Value:   []byte{'o', 'k'},
		Session: sess,
	}

	c.infof("created")

	// renew in the background
	go func() {
		if err = c.api.Session().RenewPeriodic(ttl, sess, nil, c.stopCh); err != nil {
			c.infof("renew session error: %v", err)
		}
	}()

	// destroy session when the stopCh is closed
	go func() {
		<-c.stopCh
		if err := c.destroySession(); err != nil {
			c.infof("destroy session error: %v", err)
		}
	}()

	// acquire lock
	c.infof("lock")

	var waitIndex uint64

	for {
		kv, _, err := c.api.KV().Get(lockKey, &api.QueryOptions{
			WaitTime:  waitTime,
			WaitIndex: waitIndex,
		})

		if err != nil {
			return err
		}

		if kv != nil {
			waitIndex = kv.ModifyIndex
		}

		ok, _, err := c.api.KV().Acquire(c.lock, nil)
		if err != nil {
			return err
		}

		if ok {
			c.infof("acquired")
			c.locked = true
			break
		}
	}

	return nil
}

// destroySession destroys consul agent session
func (c *Consul) destroySession() error {
	if c.locked {
		c.infof("release")
		_, _, err := c.api.KV().Release(c.lock, nil)
		if err != nil {
			return err
		}
	}

	// destroy session
	c.infof("destroy")
	_, err := c.api.Session().Destroy(c.lock.Session, nil)
	if err != nil {
		return err
	}
	return nil
}

// Next returns slices of critical and passing events
func (c *Consul) Next() (api.HealthChecks, error) {
	for {
		select {
		case ev := <-c.nextCh:
			if ev == nil {
				return nil, ErrClosed
			}
			return ev.hcs, ev.err
		case <-c.stopCh:
			return nil, ErrClosed
		}
	}
}

type payload struct {
	hcs api.HealthChecks
	err error
}

func (c *Consul) watch() {
	defer close(c.nextCh)

	// load state
	curr, err := c.load()
	if err != nil {
		c.infof("load state error %v", err)
	}
	c.infof("state is %v", curr)

	next := api.HealthChecks{}
	meta := &api.QueryMeta{}

	for {
		select {
		case <-c.stopCh:
			break
		default:
		}

		hcks := api.HealthChecks{}
		next, meta, err = c.api.Health().State("any", &api.QueryOptions{
			AllowStale: true,
			WaitIndex:  meta.LastIndex,
			WaitTime:   waitTime,
		})

		if err != nil {
			c.nextCh <- &payload{err: err}
			break
		}

		// we track modified service states to avoid multiple notifications
		// on the same service with the same status
		mods := map[string]bool{}

	Loop:
		for _, check := range next {
			// skip self check
			if check.ServiceID == "" {
				continue
			}

			for _, ch := range curr {
				if check.ServiceID == ch.ServiceID {
					if check.Status == ch.Status || (check.Status != ch.Status && mods[check.ServiceID]) {
						continue Loop
					}

					// service status has changed
					mods[check.ServiceID] = true
					break
				}
			}

			hcks = append(hcks, check)
			c.infof("%s:%s %s", check.Node, check.ServiceID, check.Status)
		}

		// send data only when we have any
		if len(hcks) == 0 {
			continue
		}

		c.nextCh <- &payload{hcs: hcks}

		// save state
		curr = next
		if err = c.dump(curr); err != nil {
			c.nextCh <- &payload{err: err}
			break
		}
	}
}

// load loads consul state from kv store
func (c *Consul) load() (api.HealthChecks, error) {
	kv, _, err := c.api.KV().Get(stateKey, nil)
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

	_, err = c.api.KV().Put(&api.KVPair{
		Key:   stateKey,
		Value: b,
	}, nil)

	return err
}

// Close shuts down Next() function
func (c *Consul) Close() error {
	close(c.stopCh)
	return nil
}

// infof prints a debug message to stderr when debug mode is enabled
func (c *Consul) infof(s string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, "consul: "+s+"\n", v...)
}
