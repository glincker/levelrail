package deploy

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RolloutPolicyStore persists a service's rollout: block. *store.DB satisfies it.
type RolloutPolicyStore interface {
	SetRolloutPolicy(ctx context.Context, p store.RolloutPolicy) error
	DeleteRolloutPolicy(ctx context.Context, serviceName string) error
}

// WithRolloutPolicyStore makes a deploy save the spec's rollout: block, or
// clear it when the spec drops the block.
func WithRolloutPolicyStore(s RolloutPolicyStore) Option {
	return func(p *Pipeline) { p.rolloutPolicies = s }
}

func (p *Pipeline) saveRolloutPolicy(ctx context.Context, req Request) error {
	if p.rolloutPolicies == nil {
		return nil
	}
	r := req.Service.Rollout
	if r == nil {
		if err := p.rolloutPolicies.DeleteRolloutPolicy(ctx, req.ServiceName); err != nil {
			return fmt.Errorf("deploy: service %q: clear rollout policy: %w", req.ServiceName, err)
		}
		return nil
	}
	if err := r.Validate(req.ServiceName); err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	err := p.rolloutPolicies.SetRolloutPolicy(ctx, store.RolloutPolicy{
		ServiceName: req.ServiceName, MaxSurge: r.MaxSurge, MaxUnavailable: r.MaxUnavailable, MinAvailable: r.MinAvailable,
	})
	if err != nil {
		return fmt.Errorf("deploy: service %q: save rollout policy: %w", req.ServiceName, err)
	}
	return nil
}
