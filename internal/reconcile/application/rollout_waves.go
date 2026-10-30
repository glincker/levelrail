package application

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RolloutPolicySource reads an app's saved rollout: block. A ServiceStore
// that also implements it (*store.DB does) gets per-app surge settings.
type RolloutPolicySource interface {
	GetRolloutPolicy(ctx context.Context, serviceName string) (store.RolloutPolicy, bool, error)
}

func (c *Controller) rolloutFor(ctx context.Context, replicas int) rolloutpolicy.Resolved {
	var pol rolloutpolicy.Policy
	if src, ok := c.store.(RolloutPolicySource); ok {
		p, found, err := src.GetRolloutPolicy(ctx, c.serviceName)
		if err != nil {
			slog.WarnContext(ctx, "application: rollout policy unreadable, using env defaults", "service", c.serviceName, "error", err)
		} else if found {
			pol = rolloutpolicy.Policy{MaxSurge: p.MaxSurge, MaxUnavailable: p.MaxUnavailable, MinAvailable: p.MinAvailable}
		}
	}
	return rolloutpolicy.Resolve(pol, rolloutpolicy.OSEnv, replicas)
}

// reconcileRolling replaces targets in waves of max_surge+max_unavailable:
// up to max_unavailable old replicas retire before a wave starts, the
// wave's replicas start and pass readiness, then the rest of the wave's
// old replicas retire. Healthy replicas never drop below desired minus
// max_unavailable, and extra containers stay within max_surge
// (best-effort while a drain window or previous-release hold keeps old
// replicas). A replica that fails readiness stops the pass, so the
// crash-loop rollback path runs exactly as it does for one-at-a-time.
func (c *Controller) reconcileRolling(ctx context.Context, targets []string, desired *store.DesiredService) (reconcile.Result, error) {
	res := c.rolloutFor(ctx, len(targets))
	step := res.Step()
	waves := res.Waves(len(targets))
	prog := rolloutpolicy.Progress{
		Active: true, Phase: rolloutpolicy.PhaseRolling, Waves: waves, Desired: len(targets),
		MaxSurge: res.MaxSurge, MaxUnavailable: res.MaxUnavailable,
	}
	anyDeployed := false
	for w := 0; w*step < len(targets); w++ {
		lo, hi := w*step, min(len(targets), (w+1)*step)
		wave := targets[lo:hi]
		pre := 0
		prog.Wave = w + 1
		if res.MaxUnavailable > 0 {
			if missing := c.missingTargets(ctx, wave); missing > 0 {
				pre = c.retireOld(ctx, targets, desired, min(res.MaxUnavailable, missing))
				c.noteProgress(ctx, targets, lo, &prog)
			}
		}
		deployedInWave := 0
		for j, target := range wave {
			deployed, err := c.ensureReplicaRunning(ctx, target, lo+j, desired)
			if err != nil {
				prog.Phase, prog.Active, prog.Message = rolloutpolicy.PhaseFailed, false, err.Error()
				c.noteProgress(ctx, targets, lo+j, &prog)
				return notReady(deployed.reason, err), fmt.Errorf("application/%s: %w", c.serviceName, err)
			}
			if deployed.justDeployed {
				deployedInWave++
			}
		}
		if deployedInWave > 0 {
			anyDeployed = true
			if rest := deployedInWave - pre; rest > 0 {
				c.retireOld(ctx, targets, desired, rest)
			}
			c.noteProgress(ctx, targets, hi, &prog)
		}
	}

	if err := c.retireStale(ctx, targets, desired, anyDeployed); err != nil {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue,
			Reason: "RunningStaleCleanupFailed", Message: err.Error(),
		}}}, fmt.Errorf("application/%s: cleanup stale containers: %w", c.serviceName, err)
	}
	if anyDeployed {
		prog.Phase, prog.Active, prog.Message = rolloutpolicy.PhaseComplete, false, ""
		c.noteProgress(ctx, targets, len(targets), &prog)
	}
	return c.finishReconcile(ctx, targets, desired, anyDeployed)
}

// missingTargets counts wave targets with no container yet; names derive
// from image plus index, so a new image makes every target missing.
func (c *Controller) missingTargets(ctx context.Context, wave []string) int {
	n := 0
	for _, t := range wave {
		if st, err := c.inspectByName(ctx, t); err == nil && st == nil {
			n++
		}
	}
	return n
}

// retireOld removes up to n stale containers under the same rules the
// one-at-a-time pass used: nothing old stops inside a drain window, the
// held previous release keeps one, and an old replica 0 goes last since
// ingress still dials it until the pass ends.
func (c *Controller) retireOld(ctx context.Context, targets []string, desired *store.DesiredService, n int) int {
	stale, err := c.staleContainers(ctx, targets)
	if err != nil || n <= 0 {
		return 0
	}
	safety := c.safetyFor(ctx, desired)
	if safety.DrainTimeout > 0 {
		return 0
	}
	hold := c.effectiveHold(safety) > 0
	// Created ties at fake-clock resolution, so break ties on name.
	sort.SliceStable(stale, func(i, j int) bool {
		if !stale[i].Created.Equal(stale[j].Created) {
			return stale[i].Created.Before(stale[j].Created)
		}
		return stale[i].Name < stale[j].Name
	})
	removed := 0
	left := len(stale)
	for _, cs := range stale {
		if removed >= n || left == 0 || (left == 1 && hold) {
			break
		}
		if releaseOf(cs.Name) == cs.Name {
			continue
		}
		if c.removeContainers(ctx, []docker.ContainerState{cs}) == nil {
			removed++
			left--
		}
	}
	return removed
}

// noteProgress publishes counts for the dashboard and CLI: updated is how
// many targets are already on the new release.
func (c *Controller) noteProgress(ctx context.Context, targets []string, updated int, p *rolloutpolicy.Progress) {
	old := 0
	if stale, err := c.staleContainers(ctx, targets); err == nil {
		old = len(stale)
	}
	p.Updated = updated
	p.Ready = min(p.Desired, updated+old)
	p.Unavailable = max(0, p.Desired-p.Ready)
	p.Surge = max(0, updated+old-p.Desired)
	rolloutpolicy.Record(c.serviceName, *p)
}
