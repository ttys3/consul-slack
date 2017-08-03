package consul

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

const stateKey = "consul-slack/state"

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

	go c.watch()
	return c, nil
}

// Consul is the consul server client
type Consul struct {
	api *api.Client
	mu  sync.Mutex

	stopCh    chan struct{}
	stoppedCh chan struct{}

	// C is channel that events are pushed to
	C chan *Event

	address    string
	scheme     string
	datacenter string
	logger     *log.Logger
}

var waitTime = 15 * time.Second

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

// watches for changes and sends them to C.
func (c *Consul) watch() {
	defer close(c.C)

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
			close(c.stoppedCh)
			return
		default:
		}

		data, meta, err = c.api.Health().State("any", &api.QueryOptions{
			AllowStale: true,
			WaitIndex:  meta.LastIndex,
			WaitTime:   waitTime,
		})

		if err != nil {
			c.C <- &Event{Err: err}
			break
		}

		next := mkState(data)
		for id, status := range next {
			if curr[id] == status {
				continue
			}

			c.infof("%s: %s", id, status)

			chunks := strings.SplitN(id, ":", 2)
			c.C <- &Event{
				Node:      chunks[0],
				ServiceID: chunks[1],
				Status:    status,
			}
		}

		// save state
		curr = next
		if err = c.dump(curr); err != nil {
			c.C <- &Event{Err: err}
			break
		}
	}
}

// TODO: implement Added and Deleted states.
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

// Event is a service state change.
type Event struct {
	Node      string
	ServiceID string
	Status    string
	Err       error
}

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

// infof prints a debug message when debug mode is enabled.
func (c *Consul) infof(format string, v ...interface{}) {
	if c.logger != nil {
		c.logger.Printf(format, v...)
	}
}
