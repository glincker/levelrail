package spec

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

// LoadBalancerFor returns a service's loadbalancer: block, or nil when the
// service has none.
func (s *Spec) LoadBalancerFor(service string) (*loadbalancer.Config, error) {
	svc, ok := s.Services[service]
	if !ok {
		return nil, fmt.Errorf("spec: no service %q", service)
	}
	return svc.LoadBalancer, nil
}
