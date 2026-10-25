package application

import (
	"context"
	"log/slog"
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
		if cs.Running && ownsContainer(c.serviceName, cs.Name) {
			return
		}
	}
	attempts, err := hist.ListDeployAttempts(ctx, c.serviceName)
	if err != nil {
		return
	}
	rank := map[string]int{}
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded || a.Image == "" || a.Image == desired.Image {
			continue
		}
		if _, seen := rank[a.Image]; !seen {
			rank[a.Image] = len(rank)
		}
	}
	var pick *docker.ContainerState
	for i := range others {
		cs := &others[i]
		r, known := rank[cs.Image]
		if !known || cs.Running {
			continue
		}
		if pick == nil || r < rank[pick.Image] {
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
