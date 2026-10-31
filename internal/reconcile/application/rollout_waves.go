package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"

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

// RolloutProgressStore persists rollout progress across control plane
// restarts. A ServiceStore that implements it (*store.DB does) is used.
type RolloutProgressStore interface {
	SaveRolloutProgress(ctx context.Context, serviceName string, payload []byte) error
	LoadRolloutProgress(ctx context.Context, serviceName string) ([]byte, bool, error)
}

// hydrateProgress restores the saved progress after a restart. It reports
// whether the restored rollout was still marked active.
func (c *Controller) hydrateProgress(ctx context.Context) bool {
	ps, ok := c.store.(RolloutProgressStore)
	if !ok {
		return false
	}
	if _, have := rolloutpolicy.Lookup(c.serviceName); have {
		return false
	}
	raw, found, err := ps.LoadRolloutProgress(ctx, c.serviceName)
	if err != nil || !found {
		return false
	}
	var p rolloutpolicy.Progress
	if json.Unmarshal(raw, &p) != nil {
		return false
	}
	rolloutpolicy.Record(c.serviceName, p)
	return p.Active
}

func (c *Controller) persistProgress(ctx context.Context) {
	ps, ok := c.store.(RolloutProgressStore)
	if !ok {
		return
	}
	p, have := rolloutpolicy.Lookup(c.serviceName)
	if !have {
		return
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	if err := ps.SaveRolloutProgress(ctx, c.serviceName, raw); err != nil {
		slog.WarnContext(ctx, "application: rollout progress not saved", "service", c.serviceName, "error", err)
	}
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
	resumed := c.hydrateProgress(ctx)
	step := res.Step()
	waves := res.Waves(len(targets))
	prog := rolloutpolicy.Progress{
		Active: true, Phase: rolloutpolicy.PhaseRolling, Waves: waves, Desired: len(targets),
		MaxSurge: res.MaxSurge, MaxUnavailable: res.MaxUnavailable,
	}
	// Live running replicas never drop below this, whatever the counts say.
	floor := max(res.MinAvailable, len(targets)-res.MaxUnavailable)
	anyDeployed := false
	for w := 0; w*step < len(targets); w++ {
		lo, hi := w*step, min(len(targets), (w+1)*step)
		wave := targets[lo:hi]
		pre := 0
		prog.Wave = w + 1
		if res.MaxUnavailable > 0 {
			if missing := c.missingTargets(ctx, wave); missing > 0 {
				pre = c.retireOld(ctx, targets, desired, min(res.MaxUnavailable, missing), floor)
				c.noteProgress(ctx, targets, lo, &prog)
			}
		}
		deployedInWave, werr := c.startWave(ctx, wave, lo, desired)
		if werr != nil {
			prog.Phase, prog.Active, prog.Message = rolloutpolicy.PhaseFailed, false, werr.err.Error()
			c.noteProgress(ctx, targets, lo+werr.at, &prog)
			return notReady(werr.reason, werr.err), fmt.Errorf("application/%s: %w", c.serviceName, werr.err)
		}
		if deployedInWave > 0 {
			anyDeployed = true
			if rest := deployedInWave - pre; rest > 0 {
				c.retireOld(ctx, targets, desired, rest, floor)
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
	if anyDeployed || resumed {
		prog.Phase, prog.Active, prog.Message = rolloutpolicy.PhaseComplete, false, ""
		c.noteProgress(ctx, targets, len(targets), &prog)
	}
	return c.finishReconcile(ctx, targets, desired, anyDeployed)
}

type waveFailure struct {
	at     int
	reason string
	err    error
}

// startWave brings a wave's replicas up. Replica 0 runs its pre-deploy hook
// so it goes first alone; the rest start in parallel so max surge buys
// speed. Pinned host ports stay sequential since the handoff is per port.
func (c *Controller) startWave(ctx context.Context, wave []string, lo int, desired *store.DesiredService) (int, *waveFailure) {
	outs := make([]replicaOutcome, len(wave))
	errs := make([]error, len(wave))
	first := 0
	parallel := len(wave) > 1 && desired.HostPort == nil
	if parallel && lo == 0 {
		outs[0], errs[0] = c.ensureReplicaRunning(ctx, wave[0], 0, desired)
		first = 1
		if errs[0] != nil {
			return 0, &waveFailure{at: 0, reason: outs[0].reason, err: errs[0]}
		}
	}
	if parallel {
		var wg sync.WaitGroup
		for j := first; j < len(wave); j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				outs[j], errs[j] = c.ensureReplicaRunning(ctx, wave[j], lo+j, desired)
			}()
		}
		wg.Wait()
	} else {
		for j := range wave {
			outs[j], errs[j] = c.ensureReplicaRunning(ctx, wave[j], lo+j, desired)
			if errs[j] != nil {
				break
			}
		}
	}
	n := 0
	for j := range wave {
		if errs[j] != nil {
			return n, &waveFailure{at: j, reason: outs[j].reason, err: errs[j]}
		}
		if outs[j].justDeployed {
			n++
		}
	}
	return n, nil
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
func (c *Controller) retireOld(ctx context.Context, targets []string, desired *store.DesiredService, n, floor int) int {
	stale, err := c.staleContainers(ctx, targets)
	if err != nil || n <= 0 {
		return 0
	}
	live := c.liveRunning(ctx)
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
		if cs.Running && live-1 < floor {
			continue
		}
		if c.removeContainers(ctx, []docker.ContainerState{cs}) == nil {
			removed++
			left--
			if cs.Running {
				live--
			}
		}
	}
	return removed
}

// liveRunning counts this app's containers that are running right now, so a
// crashed replica never counts toward the availability floor.
func (c *Controller) liveRunning(ctx context.Context) int {
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return 0
	}
	n := 0
	for _, cs := range c.filterStale(all, nil) {
		if cs.Running {
			n++
		}
	}
	return n
}

// noteProgress publishes counts for the dashboard and CLI: updated is how
// many targets are already on the new release.
func (c *Controller) noteProgress(ctx context.Context, targets []string, updated int, p *rolloutpolicy.Progress) {
	old, oldRunning := 0, 0
	if stale, err := c.staleContainers(ctx, targets); err == nil {
		old = len(stale)
		for _, cs := range stale {
			if cs.Running {
				oldRunning++
			}
		}
	}
	p.Updated = updated
	p.Ready = min(p.Desired, updated+oldRunning)
	p.Unavailable = max(0, p.Desired-p.Ready)
	p.Surge = max(0, updated+old-p.Desired)
	rolloutpolicy.Record(c.serviceName, *p)
	c.persistProgress(ctx)
}
