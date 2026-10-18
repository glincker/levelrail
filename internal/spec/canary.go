package spec

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/canarypolicy"
)

// Canary is a service's staged canary policy (internal/canary). Unset fields
// fall back to the APP_CANARY_* defaults.
type Canary struct {
	Stages []int `yaml:"stages,omitempty"`
	// Interval is how long each stage observes traffic, Go duration syntax.
	Interval     string  `yaml:"interval,omitempty"`
	MaxErrorRate float64 `yaml:"max_error_rate,omitempty"`
	MaxP95Ratio  float64 `yaml:"max_p95_ratio,omitempty"`
}

// Policy overlays the block on base and validates the result.
func (c *Canary) Policy(base canarypolicy.Policy) (canarypolicy.Policy, error) {
	p := base
	if c != nil {
		if len(c.Stages) > 0 {
			p.Stages = append([]int(nil), c.Stages...)
		}
		if c.Interval != "" {
			d, err := time.ParseDuration(c.Interval)
			if err != nil || d < time.Second {
				return p, fmt.Errorf("canary.interval: %q is not a duration of at least 1s", c.Interval)
			}
			p.Interval = d
		}
		if c.MaxErrorRate != 0 {
			p.MaxErrorRate = c.MaxErrorRate
		}
		if c.MaxP95Ratio != 0 {
			p.MaxP95Ratio = c.MaxP95Ratio
		}
	}
	if err := p.Validate(); err != nil {
		return p, fmt.Errorf("canary: %w", err)
	}
	return p, nil
}

func (svc *Service) validateCanary(name string) error {
	if svc.Canary == nil && svc.Strategy != StrategyCanary {
		return nil
	}
	if _, err := svc.Canary.Policy(canarypolicy.DefaultPolicy(canarypolicy.OSEnv)); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	return nil
}
