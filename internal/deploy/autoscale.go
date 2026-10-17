package deploy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AutoscaleStore persists a service's autoscale policy and reads the replica
// count the autoscaler last wrote. *store.DB satisfies this structurally.
type AutoscaleStore interface {
	GetAutoscalePolicy(ctx context.Context, app string) (autoscale.Policy, error)
	SaveAutoscalePolicy(ctx context.Context, p autoscale.Policy, ev autoscale.Event, now time.Time) error
	AutoscaleReplicas(ctx context.Context, app string) (int, bool, error)
}

// WithAutoscaleStore makes a deploy persist the spec's autoscale: block and
// keep the autoscaler's replica count. ceiling is the global maximum replicas.
// A spec with no block leaves any dashboard or CLI configured policy alone.
func WithAutoscaleStore(s AutoscaleStore, ceiling int) Option {
	return func(p *Pipeline) { p.autoscale, p.autoscaleCeiling = s, ceiling }
}

func (p *Pipeline) autoscalePolicy(ctx context.Context, req Request) (autoscale.Policy, bool, error) {
	if p.autoscale == nil {
		return autoscale.Policy{}, false, nil
	}
	if req.Service.Autoscale != nil {
		pol, err := req.Service.Autoscale.Policy(req.ServiceName, p.autoscaleCeiling)
		return pol, err == nil, err
	}
	pol, err := p.autoscale.GetAutoscalePolicy(ctx, req.ServiceName)
	if errors.Is(err, autoscale.ErrNotFound) {
		return pol, false, nil
	}
	if err != nil {
		return pol, false, fmt.Errorf("read autoscale policy: %w", err)
	}
	if req.Service.Replicas != 0 {
		return pol, false, errors.New("replicas is managed by autoscale: remove replicas from the spec, or clear the autoscale policy first")
	}
	return pol, true, nil
}

// applyAutoscaleReplicas keeps a deploy from resetting the replica count the
// autoscaler owns; a service with no policy is untouched.
func (p *Pipeline) applyAutoscaleReplicas(ctx context.Context, req Request, desired *store.DesiredService) error {
	pol, ok, err := p.autoscalePolicy(ctx, req)
	if err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	if !ok {
		return nil
	}
	replicas, _, err := p.autoscale.AutoscaleReplicas(ctx, req.ServiceName)
	if err != nil && !errors.Is(err, autoscale.ErrNotFound) {
		return fmt.Errorf("deploy: service %q: read replicas: %w", req.ServiceName, err)
	}
	desired.Replicas = min(max(replicas, pol.MinReplicas), pol.MaxReplicas)
	return nil
}

func (p *Pipeline) saveAutoscale(ctx context.Context, req Request) error {
	if p.autoscale == nil || req.Service.Autoscale == nil {
		return nil
	}
	pol, err := req.Service.Autoscale.Policy(req.ServiceName, p.autoscaleCeiling)
	if err != nil {
		return fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	now := time.Now()
	ev := autoscale.Event{App: req.ServiceName, Kind: autoscale.EventConfigured, At: now, Detail: "set from app.yaml"}
	if err := p.autoscale.SaveAutoscalePolicy(ctx, pol, ev, now); err != nil {
		return fmt.Errorf("deploy: service %q: save autoscale policy: %w", req.ServiceName, err)
	}
	return nil
}
