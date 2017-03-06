package consul

import (
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

	return &Consul{
		kvAPI:     c.KV(),
		healthAPI: c.Health(),
	}, nil
}

// Config is consul configuration
type Config struct {
	Address    string
	Scheme     string
	Datacenter string
}

// Consul is the consul server client
type Consul struct {
	kvAPI     *api.KV
	healthAPI *api.Health

	// TODO: use KV
	// cc is critical checks
	cc api.HealthChecks
}

// Next returns slices of critical and passing events
func (c *Consul) Next() (cc api.HealthChecks, pc api.HealthChecks, err error) {
	hc, _, err := c.healthAPI.State("critical", nil)
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
	}

	// critical
	for _, check := range hc {
		if pos(c.cc, check) != -1 {
			continue
		}

		cc = append(cc, check)
		c.cc = append(c.cc, check)
	}

	return
}

// pos finds check's position in health checks slice or return -1
func pos(hcs api.HealthChecks, hc *api.HealthCheck) int {
	for i, c := range hcs {
		if c.CheckID == hc.CheckID {
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
