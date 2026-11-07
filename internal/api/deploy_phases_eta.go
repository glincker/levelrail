package api

import (
	"context"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// eta_basis values on the phases response.
const (
	etaBasisApp      = "app_history"
	etaBasisPlatform = "platform_history"
	etaBasisNone     = "none"

	// A baseline below this many successful deploys is noise, so it is not used.
	etaMinSamples = 2
	// Platform history reads a wider window of build attempts than the app limit, since failures are skipped.
	platformWindowFactor = 6
)

// phaseBaseline is the median total and per-phase duration of past successes.
type phaseBaseline struct {
	total    int64
	perPhase map[string]int64
	n        int
}

// medianMS is the median of v; the middle value resists a single slow outlier. Empty input gives 0.
func medianMS(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// buildBaseline medians up to limit successful attempts (newest first), skipping exclude. ok is false below etaMinSamples.
func (rt *Router) buildBaseline(ctx context.Context, attempts []store.DeployAttempt, exclude string, limit int) (phaseBaseline, bool) {
	var totals []int64
	byPhase := map[string][]int64{}
	for _, s := range attempts {
		if len(totals) >= limit {
			break
		}
		if s.ID == exclude || s.Status != store.DeployAttemptStatusSucceeded || s.FinishedAt == nil {
			continue
		}
		totals = append(totals, s.FinishedAt.Sub(s.StartedAt).Milliseconds())
		var logs []telemetry.DeployLogEntry
		if rt.deployLogStore != nil {
			logs, _ = rt.deployLogStore.QueryDeployLog(ctx, s.ID)
		}
		for _, p := range derivePhases(s, rt.traceRows(ctx, s.ID), logs, *s.FinishedAt) {
			if p.DurationMS != nil && *p.DurationMS > 0 {
				byPhase[p.Phase] = append(byPhase[p.Phase], *p.DurationMS)
			}
		}
	}
	b := phaseBaseline{n: len(totals), perPhase: map[string]int64{}}
	if len(totals) < etaMinSamples {
		return b, false
	}
	for k, v := range byPhase {
		if len(v) >= etaMinSamples {
			b.perPhase[k] = medianMS(v)
		}
	}
	b.total = medianMS(totals)
	return b, true
}

// pickBaseline prefers the app's own history and falls back to the platform's.
func (rt *Router) pickBaseline(ctx context.Context, a store.DeployAttempt) (phaseBaseline, string) {
	limit := rt.phaseSampleCount(ctx)
	if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, a.ServiceName); err == nil {
		if b, ok := rt.buildBaseline(ctx, attempts, a.ID, limit); ok {
			return b, etaBasisApp
		}
	}
	if rt.buildPerf != nil {
		if attempts, err := rt.buildPerf.ListBuildAttempts(ctx, limit*platformWindowFactor); err == nil {
			if b, ok := rt.buildBaseline(ctx, attempts, a.ID, limit); ok {
				return b, etaBasisPlatform
			}
		}
	}
	return phaseBaseline{}, etaBasisNone
}

// applyETA fills the baseline and ETA fields; the remaining time is the per-phase
// sum when every unfinished phase has a typical time, else total minus elapsed.
func (rt *Router) applyETA(ctx context.Context, a store.DeployAttempt, resp *deployPhasesResponse) {
	resp.ETABasis = etaBasisNone
	b, basis := rt.pickBaseline(ctx, a)
	if basis == etaBasisNone {
		return
	}
	resp.ETABasis, resp.Samples, resp.TypicalTotalMS = basis, b.n, &b.total
	for i := range resp.Phases {
		if v, has := b.perPhase[resp.Phases[i].Phase]; has {
			resp.Phases[i].TypicalMS = &v
		}
	}
	if a.FinishedAt != nil {
		return
	}
	remaining, complete := int64(0), true
	for i := range resp.Phases {
		p := &resp.Phases[i]
		if p.Status != phaseStatusPending && p.Status != phaseStatusRunning {
			continue
		}
		if p.TypicalMS == nil {
			complete = false
			continue
		}
		left := *p.TypicalMS
		if p.Status == phaseStatusRunning && p.DurationMS != nil {
			left = max(left-*p.DurationMS, 0)
		}
		s := msToSeconds(left)
		p.ETASeconds = &s
		remaining += left
	}
	if !complete {
		remaining = max(b.total-resp.ElapsedMS, 0)
	}
	eta, secs := remaining, msToSeconds(remaining)
	resp.ETAMS, resp.ETASeconds = &eta, &secs
}

func msToSeconds(ms int64) int64 {
	return int64((time.Duration(ms) * time.Millisecond).Round(time.Second) / time.Second)
}
