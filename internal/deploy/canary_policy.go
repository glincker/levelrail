package deploy

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/canarypolicy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CanaryPolicyStore persists a service's saved canary defaults. *store.DB satisfies it.
type CanaryPolicyStore interface {
	SetCanaryPolicy(ctx context.Context, p store.CanaryPolicy) error
	DeleteCanaryPolicy(ctx context.Context, serviceName string) error
}

// WithCanaryPolicyStore makes a deploy save the spec's canary: block, or clear
// it when the spec no longer asks for strategy: canary.
func WithCanaryPolicyStore(s CanaryPolicyStore) Option {
	return func(p *Pipeline) { p.canaryPolicies = s }
}

func (p *Pipeline) saveCanaryPolicy(ctx context.Context, req Request) error {
	if p.canaryPolicies == nil {
		return nil
	}
	if req.Service.Strategy != spec.StrategyCanary && req.Service.Canary == nil {
		if err := p.canaryPolicies.DeleteCanaryPolicy(ctx, req.ServiceName); err != nil {
			return fmt.Errorf("deploy: service %q: clear canary policy: %w", req.ServiceName, err)
		}
		return nil
	}
	pol, err := req.Service.Canary.Policy(canarypolicy.DefaultPolicy(canarypolicy.OSEnv))
	if err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	err = p.canaryPolicies.SetCanaryPolicy(ctx, store.CanaryPolicy{
		ServiceName: req.ServiceName, Stages: pol.Stages, IntervalSecs: int(pol.Interval.Seconds()),
		MaxErrorRate: pol.MaxErrorRate, MaxP95Ratio: pol.MaxP95Ratio,
	})
	if err != nil {
		return fmt.Errorf("deploy: service %q: save canary policy: %w", req.ServiceName, err)
	}
	return nil
}
