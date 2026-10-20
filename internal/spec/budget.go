package spec

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// Budget is a service's monthly egress and cost guardrail (internal/spendcap).
type Budget struct {
	// Egress is a size such as 500GiB or 2TB.
	Egress      string  `yaml:"egress,omitempty"`
	Cost        float64 `yaml:"cost,omitempty"`
	Action      string  `yaml:"action,omitempty"`
	Thresholds  []int   `yaml:"thresholds,omitempty"`
	ThrottleRPS int     `yaml:"throttle_rps,omitempty"`
}

// Cap converts the block into a normalized, validated cap for app.
func (b *Budget) Cap(app string) (spendcap.Cap, error) {
	c := spendcap.Cap{App: app, MonthlyCostCeiling: b.Cost, Action: b.Action, Thresholds: b.Thresholds, ThrottleRPS: b.ThrottleRPS}
	if b.Egress != "" {
		n, err := spendcap.ParseBytes(b.Egress)
		if err != nil {
			return c, fmt.Errorf("budget.egress: %w", err)
		}
		c.MonthlyEgressBytes = n
	}
	c = c.Normalize()
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("budget: %w", err)
	}
	return c, nil
}

// Validate checks the block; a nil block is valid.
func (b *Budget) Validate(service string) error {
	if b == nil {
		return nil
	}
	if _, err := b.Cap(service); err != nil {
		return fmt.Errorf("spec: service %q: %w", service, err)
	}
	return nil
}

// BudgetFor returns a service's budget: block, or nil when it has none.
func (s *Spec) BudgetFor(service string) (*Budget, error) {
	svc, ok := s.Services[service]
	if !ok {
		return nil, fmt.Errorf("spec: no service %q", service)
	}
	return svc.Budget, nil
}
