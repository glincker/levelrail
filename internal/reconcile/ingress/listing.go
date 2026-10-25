package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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

// attemptMemo shares each service's deploy attempts across one Reconcile.
type attemptMemo struct {
	mu        sync.Mutex
	byService map[string][]store.DeployAttempt
}

// deployAttempts returns serviceName's attempts, newest first. ok is false
// when the store cannot list them or the listing failed.
func (c *Controller) deployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, bool) {
	l, ok := c.store.(attemptLister)
	if !ok {
		return nil, false
	}
	memo := c.attempts.Load()
	if memo != nil {
		memo.mu.Lock()
		defer memo.mu.Unlock()
		if attempts, seen := memo.byService[serviceName]; seen {
			return attempts, true
		}
	}
	attempts, err := l.ListDeployAttempts(ctx, serviceName)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: list deploy attempts failed",
			slog.String("service", serviceName), slog.String("error", err.Error()))
		return nil, false
	}
	if memo != nil {
		memo.byService[serviceName] = attempts
	}
	return attempts, true
}

// readyForNewestAttempt trusts a Ready condition only when it was written after
// the newest attempt started: an older one can belong to the release that
// served before a rollback. A zero time means the store does not record it.
func (c *Controller) readyForNewestAttempt(ctx context.Context, serviceName string, readyAt time.Time) bool {
	if readyAt.IsZero() {
		return true
	}
	attempts, ok := c.deployAttempts(ctx, serviceName)
	if !ok || len(attempts) == 0 {
		return true
	}
	return readyAt.After(attempts[0].StartedAt)
}

// warmingImageID is the content a not yet ready target runs, so a held
// release with the same content can serve while it warms up.
func warmingImageID(svc store.DesiredService, state *docker.ContainerState) string {
	if state != nil && state.ImageID != "" {
		return state.ImageID
	}
	return svc.LocalImageID()
}
