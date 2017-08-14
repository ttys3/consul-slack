package consul

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	stateKey = "consul-slack/state"
	lockKey  = "consul-slack/.lock"
)

// Option is a configuration option.
type Option func(c *Consul)

// WithAddress sets consul address.
func WithAddress(address string) Option {
	return func(c *Consul) {
		c.address = address
	}
}

// WithScheme sets consul connection scheme http or https.
func WithScheme(schema string) Option {
	return func(c *Consul) {
		c.scheme = schema
	}
}

// WithDatacenter sets datacenter name.
func WithDatacenter(dc string) Option {
	return func(c *Consul) {
		c.datacenter = dc
	}
}

// WithLogger sets logger.
func WithLogger(l *log.Logger) Option {
	return func(c *Consul) {
		c.logger = l
	}
}

// New creates new consul client
func New(opts ...Option) (*Consul, error) {
	c := &Consul{
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		C:         make(chan *Event),
		logger:    log.New(os.Stdout, "[consul] ", log.LstdFlags),
	}

	// apply configuration options
	for _, opt := range opts {
		opt(c)
	}

	var err error
	c.api, err = connect(c)
	if err != nil {
		return nil, err
	}

	if err = c.createSession(); err != nil {
		return nil, err
	}

	go c.watch()
	return c, nil
}

// Consul is the consul server client
type Consul struct {
	api *api.Client

	stopCh    chan struct{}
	stoppedCh chan struct{}

	// C is channel that events are pushed to
	C chan *Event

	address    string
	scheme     string
	datacenter string
	logger     *log.Logger

	mu     sync.Mutex
	lock   *api.KVPair
	locked bool

	err error
}

var (
	waitTime = 15 * time.Second
	ttl      = "30s"
)

func connect(c *Consul) (*api.Client, error) {
	a, err := api.NewClient(&api.Config{
		Address:    c.address,
		Scheme:     c.scheme,
		Datacenter: c.datacenter,
	})
	if err != nil {
		return nil, err
	}

	// check agent connection
	_, err = a.Status().Leader()
	if err != nil {
		return nil, err
	}
	return a, nil
}

// createSession creates new consul session and holds an unique lock
func (c *Consul) createSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	c.logf("session created")

	// renew in the background
	go func() {
		if err = c.api.Session().RenewPeriodic(ttl, sess, nil, c.stopCh); err != nil {
			fmt.Fprintf(os.Stderr, "renew session error: %v\n", err)
		}
	}()

	// destroy session when the stopCh is closed
	go func() {
		<-c.stopCh
		if err := c.destroySession(); err != nil {
			fmt.Fprintf(os.Stderr, "destroy session error: %v\n", err)
		}
	}()

	// acquire lock
	c.logf("try lock")

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
			c.logf("lock acquired")
			c.locked = true
			break
		}
	}

	return nil
}

// destroySession destroys consul agent session.
func (c *Consul) destroySession() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.locked {
		c.logf("session release")
		_, _, err := c.api.KV().Release(c.lock, nil)
		if err != nil {
			return err
		}
	}

	// destroy session
	c.logf("session destroy")
	_, err := c.api.Session().Destroy(c.lock.Session, nil)
	if err != nil {
		return err
	}
	return nil
}

// Err is an error encountered during iteration.
func (c *Consul) Err() error {
	return c.err
}

// watches for changes and sends them to C.
func (c *Consul) watch() {
	defer close(c.C)

	// load state
	curr, err := c.load()
	if err != nil {
		c.logf("load state error %v", err)
	}
	c.logf("state is %v", curr)

	meta := &api.QueryMeta{}
	data := api.HealthChecks{}

	for {
		select {
		case <-c.stopCh:
			close(c.stoppedCh)
			return
		default:
		}

		data, meta, err = c.api.Health().State(api.HealthAny, &api.QueryOptions{
			AllowStale: true,
			WaitIndex:  meta.LastIndex,
			WaitTime:   waitTime,
		})

		if err != nil {
			c.err = err
			return
		}

		next := make(state)
		for id, hc := range toIDMap(data) {
			// we need to store only serviceID to status map.
			next[id] = hc.Status

			// health check status hasn't changed
			if curr[id] == hc.Status {
				continue
			}

			c.logf("%s: %s", id, hc.Status)
			c.C <- (*Event)(hc)
		}

		// save state
		curr = next
		if err = c.dump(curr); err != nil {
			c.err = err
			return
		}
	}
}

const (
	// TODO
	Added   = "added"
	Deleted = "deleted"

	Passing     = api.HealthPassing
	Warning     = api.HealthWarning
	Critical    = api.HealthCritical
	Maintenance = api.HealthMaint
)

// statuses is map of status name to its weight
var statuses = map[string]int{
	Passing:     0,
	Warning:     1,
	Critical:    2,
	Maintenance: 3,
}

// state is current state
type state map[string]string

// toIDMap converts a health checks list into internal state representation.
func toIDMap(hcs api.HealthChecks) map[string]*api.HealthCheck {
	r := make(map[string]*api.HealthCheck, len(hcs))
	for _, hc := range hcs {
		// ignore serf heal status
		if hc.ServiceID == "" {
			continue
		}

		// aggregate state maintenance > critical > warning > passing
		id := hc.Node + ":" + hc.ServiceID
		if h, ok := r[id]; !ok || statuses[h.Status] < statuses[hc.Status] {
			r[id] = hc
		}
	}
	return r
}

// Event is a service state change.
type Event api.HealthCheck

// load loads consul state from the kv store.
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

// dump saves consul state to the kv store.
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

// Close closes C channel.
func (c *Consul) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.stoppedCh:
		return errors.New("already closed")
	default:
	}

	close(c.stopCh)
	<-c.stoppedCh
	return nil
}

// logf prints a debug message when debug mode is enabled.
func (c *Consul) logf(format string, v ...interface{}) {
	if c.logger != nil {
		c.logger.Printf(format, v...)
	}
}
