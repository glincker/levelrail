package api

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// autoscaleSignals reads the observed world for the autoscale controller.
type autoscaleSignals struct{ rt *Router }

type gaugeWindow struct {
	mean   float64
	newest time.Time
	last   float64
	n      int
}

func (g autoscaleSignals) gauge(ctx context.Context, app, metric string, from, to time.Time) (gaugeWindow, error) {
	samples, err := g.rt.telemetry.QueryMetrics(ctx, resourceIDForApp(app), metric, from, to)
	if err != nil {
		return gaugeWindow{}, err
	}
	var w gaugeWindow
	var sum float64
	for _, s := range samples {
		sum += s.Value
		w.n++
		if s.Timestamp.After(w.newest) {
			w.newest, w.last = s.Timestamp, s.Value
		}
	}
	if w.n > 0 {
		w.mean = sum / float64(w.n)
	}
	return w, nil
}

func staleNote(w gaugeWindow, metric string, now time.Time, maxAge time.Duration) string {
	switch {
	case w.n == 0:
		return fmt.Sprintf("no %s samples in the window", metric)
	case now.Sub(w.newest) > maxAge:
		return fmt.Sprintf("the newest %s sample is %s old", metric, now.Sub(w.newest).Round(time.Second))
	}
	return ""
}

// Metric returns the policy metric for the app, or an unknown reading with
// the reason. Anything missing or stale is unknown, never zero.
func (g autoscaleSignals) Metric(ctx context.Context, p autoscale.Policy, current int, now time.Time) (autoscale.Reading, error) {
	if g.rt.telemetry == nil {
		return autoscale.Reading{Note: "telemetry is not configured on this control plane"}, nil
	}
	window := autoscale.WindowFromEnv(os.Getenv)
	maxAge := autoscale.MetricMaxAgeFromEnv(os.Getenv)
	from := now.Add(-window)
	svc, err := g.rt.apps.GetDesiredService(ctx, p.App)
	if err != nil {
		return autoscale.Reading{}, err
	}
	switch p.Metric {
	case autoscale.MetricCPUPercent:
		w, err := g.gauge(ctx, p.App, metricCPU, from, now)
		if err != nil {
			return autoscale.Reading{}, err
		}
		if note := staleNote(w, metricCPU, now, maxAge); note != "" {
			return autoscale.Reading{Note: note}, nil
		}
		cores := 1.0
		if svc.Resources != nil && svc.Resources.NanoCPUs > 0 {
			cores = float64(svc.Resources.NanoCPUs) / 1e9
		}
		return autoscale.Reading{Value: w.mean / cores, Known: true}, nil
	case autoscale.MetricMemoryPercent:
		return g.memory(ctx, p, svc.Resources, from, now, maxAge)
	default:
		return g.requests(ctx, p, current, from, now, maxAge)
	}
}

func (g autoscaleSignals) memory(ctx context.Context, p autoscale.Policy, res *store.ServiceResources, from, now time.Time, maxAge time.Duration) (autoscale.Reading, error) {
	use, err := g.gauge(ctx, p.App, metricMemoryUsage, from, now)
	if err != nil {
		return autoscale.Reading{}, err
	}
	if note := staleNote(use, metricMemoryUsage, now, maxAge); note != "" {
		return autoscale.Reading{Note: note}, nil
	}
	var limit float64
	if res != nil {
		limit = float64(res.MemoryBytes)
	}
	if limit <= 0 {
		l, err := g.gauge(ctx, p.App, metricMemoryLimit, from, now)
		if err != nil {
			return autoscale.Reading{}, err
		}
		limit = l.last
	}
	if limit <= 0 {
		return autoscale.Reading{Note: "the app has no memory limit to measure usage against"}, nil
	}
	return autoscale.Reading{Value: use.mean / limit * 100, Known: true}, nil
}

func (g autoscaleSignals) requests(ctx context.Context, p autoscale.Policy, current int, from, now time.Time, maxAge time.Duration) (autoscale.Reading, error) {
	window := now.Sub(from)
	points, err := telemetry.QueryRequests(ctx, g.rt.telemetry, p.App, from, now, window+time.Second)
	if err != nil && len(points) == 0 {
		return autoscale.Reading{}, err
	}
	if len(points) > 0 {
		return autoscale.Reading{Value: points[0].RatePerSec / float64(max(current, 1)), Known: true}, nil
	}
	// A window with no requests reads as idle only while the container's own
	// samples prove telemetry is alive; otherwise silence is unknown.
	live, err := g.gauge(ctx, p.App, metricCPU, from, now)
	if err != nil {
		return autoscale.Reading{}, err
	}
	if note := staleNote(live, metricCPU, now, maxAge); note != "" {
		return autoscale.Reading{Note: "no requests recorded and " + note}, nil
	}
	return autoscale.Reading{Known: true}, nil
}

// Headroom reports how many of extra more replicas fit on the app's node.
func (g autoscaleSignals) Headroom(ctx context.Context, app string, extra int) (int, error) {
	rt := g.rt
	if rt.multiNode == nil {
		return -1, nil
	}
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return -1, err
	}
	per := serviceRequests(svc.Resources, 1)
	if per.CPUMilli == 0 && per.MemoryBytes == 0 {
		return -1, nil
	}
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return -1, err
	}
	home := svc.NodeID
	if home == "" {
		home = rt.localNodeID
	}
	for _, c := range cands {
		if c.ID != home {
			continue
		}
		for k := extra; k >= 1; k-- {
			e := placement.Evaluate(c, placement.Request{Needs: per.Times(int64(k)), Policy: rt.placementPolicy})
			if e.Eligible || !e.Capacity {
				return k, nil
			}
		}
		return 0, nil
	}
	return -1, nil
}

// SpendEnforced returns the spend cap action in force this month, "" when none.
func (g autoscaleSignals) SpendEnforced(ctx context.Context, app string) (string, error) {
	st, ok := g.rt.apps.(spendcap.Store)
	if !ok {
		return "", nil
	}
	state, found, err := st.GetSpendCapState(ctx, app)
	if err != nil || !found || state.Period != spendcap.Period(time.Now()) {
		return "", err
	}
	return state.Enforced, nil
}
