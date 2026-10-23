package ingress

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// containerListing answers every prefix query of one Reconcile from a single
// listing, so resolving N services costs one daemon call instead of N.
type containerListing struct {
	mu     sync.Mutex
	all    []docker.ContainerState
	loaded bool
}

func (l *containerListing) withPrefix(ctx context.Context, rt docker.Runtime, prefix string) ([]docker.ContainerState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.loaded {
		all, err := rt.ListByPrefix(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("list containers: %w", err)
		}
		l.all, l.loaded = all, true
	}
	var out []docker.ContainerState
	for _, cs := range l.all {
		if strings.HasPrefix(cs.Name, prefix) {
			out = append(out, cs)
		}
	}
	return out, nil
}

// listLocal lists local containers named with prefix, sharing the Reconcile's listing when one is active.
func (c *Controller) listLocal(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	if l := c.listing.Load(); l != nil {
		return l.withPrefix(ctx, c.runtime, prefix)
	}
	return c.runtime.ListByPrefix(ctx, prefix)
}

// localTargets are the container names dialForService will look up for the services on this node.
func (c *Controller) localTargets(services []store.DesiredService) []string {
	names := make([]string, 0, len(services))
	for _, svc := range services {
		if c.isLocalNode(svc.NodeID) {
			names = append(names, application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce))
		}
	}
	return names
}

// inspectLocal is InspectByName answered from the Reconcile's batch when it covers name.
func (c *Controller) inspectLocal(ctx context.Context, name string) (*docker.ContainerState, error) {
	if state, covered := c.inspected.Load().Lookup(name); covered {
		return state, nil
	}
	return c.runtime.InspectByName(ctx, name)
}
