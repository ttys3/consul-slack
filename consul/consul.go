package consul

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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
		Value:   []byte(sess),
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
func (c *Consul) Next() ([]*Change, error) {
	ev := <-c.nextCh
	return ev.changes, ev.err
}

type payload struct {
	changes []*Change
	err     error
}

func (c *Consul) watch() {
	defer close(c.nextCh)

	// load state
	curr, err := c.load()
	if err != nil {
		c.infof("load state error %v", err)
	}
	c.infof("state is %v", curr)

	meta := &api.QueryMeta{}
	data := api.HealthChecks{}

	for {
		select {
		case <-c.stopCh:
			c.nextCh <- &payload{err: ErrClosed}
			break
		default:
		}

		data, meta, err = c.api.Health().State("any", &api.QueryOptions{
			AllowStale: true,
			WaitIndex:  meta.LastIndex,
			WaitTime:   waitTime,
		})

		if err != nil {
			c.nextCh <- &payload{err: err}
			break
		}

		mods := []*Change{}
		next := mkState(data)

		fmt.Printf("--> %#v\n", next)

		for id, status := range next {
			if curr[id] == status {
				continue
			}

			chunks := strings.SplitN(id, ":", 2)
			mods = append(mods, &Change{
				Node:      chunks[0],
				ServiceID: chunks[1],
				Status:    status,
			})
		}

		// send data only when we have any
		if len(mods) > 0 {
			c.nextCh <- &payload{changes: mods}
		}

		// save state
		curr = next
		if err = c.dump(curr); err != nil {
			c.nextCh <- &payload{err: err}
			break
		}
	}
}

// State names.
const (
	Passing  = "passing"
	Warning  = "warning"
	Critical = "critical"
)

// statuses is map of status name to its weight
var statuses = map[string]int{
	Passing:  0,
	Warning:  1,
	Critical: 2,
}

// state is current state
type state map[string]string

// mkState converts a health checks list into internal state representation
func mkState(checks api.HealthChecks) state {
	s := make(state, len(checks))
	for _, check := range checks {
		if check.ServiceID == "" {
			continue
		}

		id := check.Node + ":" + check.ServiceID
		if status, ok := s[id]; !ok || statuses[status] < statuses[check.Status] {
			s[id] = check.Status
		}
	}
	return s
}

// Change is a service state change
type Change struct {
	Node      string
	ServiceID string
	Status    string
}

// load loads consul state from kv store
func (c *Consul) load() (state, error) {
	kv, _, err := c.api.KV().Get(stateKey, nil)
	if err != nil {
		return nil, err
	}

	s := state{}
	if kv != nil {
		err = json.Unmarshal(kv.Value, &s)
	}
	return s, err
}

// dump saves consul state to kv store
func (c *Consul) dump(s state) error {
	b, err := json.Marshal(s)
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
