package application

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// attemptHistory is the optional store surface that tells which releases
// ever succeeded. Stores without it never revive anything.
type attemptHistory interface {
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// reviveLastReady starts the newest previously succeeded release again when
// the desired release is not ready and nothing of this service is serving.
// Without it, a failed desired release plus a dead live container leaves the
// app down until an operator rolls back. Level-triggered: a running
// container of any release makes it a no-op, and the shared crash backoff
// bounds the restarts.
func (c *Controller) reviveLastReady(ctx context.Context, targets []string, desired *store.DesiredService) {
	c.reviveIfDead(ctx, targets, desired, false)
}

// reviveIfDead is reviveLastReady with one extra mode: skipIfTargetRunning is
// for the start of a pass, where a running desired container may be a healthy
// release (not yet judged) and must keep older stopped ones where they are.
func (c *Controller) reviveIfDead(ctx context.Context, targets []string, desired *store.DesiredService, skipIfTargetRunning bool) {
	// Reviving an older one-shot would rerun a finished migration.
	if desired.RunsToCompletion() {
		return
	}
	hist, ok := c.store.(attemptHistory)
	if !ok {
		return
	}
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return
	}
	others := c.filterStale(all, targets)
	for _, cs := range all {
		if !cs.Running || !ownsContainer(c.serviceName, cs.Name) {
			continue
		}
		// A different container mid-create on the pinned host port would make
		// the restarted release fail to bind and fight for the port.
		if skipIfTargetRunning && slices.Contains(targets, cs.Name) {
			return
		}
		if desired.HostPort != nil && holdsHostPort(cs, *desired.HostPort) {
			return
		}
		if holdsAnyHostPort(cs, desired.PublishedPorts) {
			return
		}
	}
	attempts, err := hist.ListDeployAttempts(ctx, c.serviceName)
	if err != nil {
		return
	}
	rank := map[string]int{}
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded || a.Image == "" || docker.SameImageRef(a.Image, desired.Image) {
			continue
		}
		key := docker.CanonicalImageRef(a.Image)
		if _, seen := rank[key]; !seen {
			rank[key] = len(rank)
		}
	}
	// A running desired release that never became ready is not serving, so it
	// must not veto reviving the last ready one.
	for _, cs := range all {
		if _, known := rank[docker.CanonicalImageRef(cs.Image)]; known && cs.Running && ownsContainer(c.serviceName, cs.Name) {
			return
		}
	}
	var pick *docker.ContainerState
	for i := range others {
		cs := &others[i]
		r, known := rank[docker.CanonicalImageRef(cs.Image)]
		if !known || cs.Running {
			continue
		}
		if pick == nil || r < rank[docker.CanonicalImageRef(pick.Image)] {
			pick = cs
		}
	}
	if pick == nil {
		return
	}
	now := time.Now()
	if wait, _ := c.restartBackoff.remaining(pick.Name, now); wait > 0 {
		return
	}
	c.restartBackoff.recordRestart(pick.Name, now)
	if err := c.ensureAppNetwork(ctx, desired); err != nil {
		slog.Warn("reconcile: revive previous release: network", slog.String("service", c.serviceName), slog.String("error", err.Error()))
		return
	}
	if err := c.startContainer(ctx, pick.ID); err != nil {
		slog.Warn("reconcile: revive previous release failed", slog.String("service", c.serviceName), slog.String("container", pick.Name), slog.String("error", err.Error()))
		return
	}
	slog.Info("reconcile: desired release not ready and nothing serving, restarted the last ready release", slog.String("service", c.serviceName), slog.String("container", pick.Name))
}

const defaultReviveWatch = 3 * time.Second

// WithReviveWatch sets how often a readiness wait checks that the last ready
// release is still alive; d <= 0 keeps the default.
func WithReviveWatch(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.reviveWatch = d }
}

func (c *Controller) effectiveReviveWatch() time.Duration {
	if c.reviveWatch > 0 {
		return c.reviveWatch
	}
	return defaultReviveWatch
}
