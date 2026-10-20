package deploy

import (
	"context"
	"encoding/json"
	"fmt"
)

// LoadBalancerStore persists a service's loadbalancer: block from app.yaml.
// *store.DB satisfies this structurally.
type LoadBalancerStore interface {
	SetServiceLoadBalancer(ctx context.Context, service, configJSON string) error
}

// WithLoadBalancerStore makes a deploy persist the spec's loadbalancer:
// block. A spec with no block leaves any UI or CLI configured balancer alone.
func WithLoadBalancerStore(s LoadBalancerStore) Option {
	return func(p *Pipeline) { p.loadBalancers = s }
}

func (p *Pipeline) saveLoadBalancer(ctx context.Context, req Request) error {
	if p.loadBalancers == nil || req.Service.LoadBalancer == nil {
		return nil
	}
	raw, err := json.Marshal(req.Service.LoadBalancer)
	if err != nil {
		return fmt.Errorf("deploy: service %q: encode loadbalancer: %w", req.ServiceName, err)
	}
	if err := p.loadBalancers.SetServiceLoadBalancer(ctx, req.ServiceName, string(raw)); err != nil {
		return fmt.Errorf("deploy: service %q: save loadbalancer: %w", req.ServiceName, err)
	}
	return nil
}
