package canary

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Pair is the ingress upstream addresses (host:port) of an app's stable
// release and its canary, as the ingress controller last routed them.
type Pair struct {
	Stable string
	Canary string
}

// Dials is the shared record of which upstream is which release.
type Dials struct {
	mu sync.RWMutex
	m  map[string]Pair
}

// NewDials returns an empty record.
func NewDials() *Dials { return &Dials{m: make(map[string]Pair)} }

// Set records app's current upstreams.
func (d *Dials) Set(app string, p Pair) {
	d.mu.Lock()
	d.m[app] = p
	d.mu.Unlock()
}

// Delete forgets app, used when its canary is not receiving traffic.
func (d *Dials) Delete(app string) {
	d.mu.Lock()
	delete(d.m, app)
	d.mu.Unlock()
}

// Get returns app's upstreams.
func (d *Dials) Get(app string) (Pair, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.m[app]
	return p, ok
}

// Store is the persistence the reconciler needs; *store.DB satisfies it.
type Store interface {
	ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error)
	SetCanaryRelease(ctx context.Context, c store.CanaryRelease) error
}

// Stats reads cumulative per-upstream counters; ingress.RequestStats satisfies it.
type Stats interface {
	UpstreamTotals() map[string]telemetry.UpstreamStat
}

// Actions are the side effects a decision can trigger. Both must be
// idempotent: a tick that fails half way is simply repeated.
type Actions interface {
	Promote(ctx context.Context, rel store.CanaryRelease) error
	Rollback(ctx context.Context, rel store.CanaryRelease, reason string) error
}

type window struct {
	image  string
	stage  int
	pair   Pair
	start  time.Time
	canary Sample
	stable Sample
}

// Reconciler converges staged canaries toward their desired stage. Desired
// state is the release row; the per-stage observation window is the only
// in-memory state and a restart just reopens it.
type Reconciler struct {
	store   Store
	stats   Stats
	dials   *Dials
	actions Actions
	limits  Limits
	now     func() time.Time
	logger  *slog.Logger

	mu      sync.Mutex
	windows map[string]*window
}

// NewReconciler builds a reconciler. now may be nil for the wall clock.
func NewReconciler(s Store, stats Stats, dials *Dials, actions Actions, limits Limits, now func() time.Time, logger *slog.Logger) *Reconciler {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{store: s, stats: stats, dials: dials, actions: actions, limits: limits, now: now, logger: logger, windows: make(map[string]*window)}
}

// Run reconciles every tick until ctx is done.
func (r *Reconciler) Run(ctx context.Context) {
	t := time.NewTicker(r.limits.Tick)
	defer t.Stop()
	for {
		if err := r.Reconcile(ctx); err != nil {
			r.logger.WarnContext(ctx, "canary: reconcile failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func policyOf(rel store.CanaryRelease) Policy {
	return Policy{Stages: rel.Stages, Interval: time.Duration(rel.IntervalSecs) * time.Second, MaxErrorRate: rel.MaxErrorRate, MaxP95Ratio: rel.MaxP95Ratio}
}

// Reconcile evaluates every staged canary once.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	releases, err := r.store.ListCanaryReleases(ctx)
	if err != nil {
		return fmt.Errorf("canary: list releases: %w", err)
	}
	live := make(map[string]struct{}, len(releases))
	totals := r.stats.UpstreamTotals()
	var firstErr error
	for _, rel := range releases {
		if !rel.Staged() {
			continue
		}
		live[rel.ServiceName] = struct{}{}
		if err := r.reconcileOne(ctx, rel, totals); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.mu.Lock()
	for app := range r.windows {
		if _, ok := live[app]; !ok {
			delete(r.windows, app)
		}
	}
	r.mu.Unlock()
	return firstErr
}

func (r *Reconciler) reconcileOne(ctx context.Context, rel store.CanaryRelease, totals map[string]telemetry.UpstreamStat) error {
	app := rel.ServiceName
	pair, ok := r.dials.Get(app)
	if !ok || rel.Weight <= 0 {
		r.drop(app)
		return nil
	}
	w := r.windowFor(rel, pair, totals)
	in := r.input(rel, w, totals)
	d := Decide(in)
	switch d.Action {
	case Advance:
		next := rel.StageIndex + 1
		rel.StageIndex, rel.Weight, rel.StatusReason = next, policyOf(rel).Weight(next), ""
		if err := r.store.SetCanaryRelease(ctx, rel); err != nil {
			return fmt.Errorf("canary: advance %q: %w", app, err)
		}
		r.drop(app)
	case Promote:
		if err := r.actions.Promote(ctx, rel); err != nil {
			rel.StatusReason = "promote failed: " + err.Error()
			if serr := r.store.SetCanaryRelease(ctx, rel); serr != nil {
				r.logger.WarnContext(ctx, "canary: record promote failure", slog.String("app", app), slog.String("error", serr.Error()))
			}
			r.drop(app)
			return fmt.Errorf("canary: promote %q: %w", app, err)
		}
		r.drop(app)
	case Rollback:
		if err := r.actions.Rollback(ctx, rel, d.Reason); err != nil {
			return fmt.Errorf("canary: roll back %q: %w", app, err)
		}
		r.drop(app)
	}
	return nil
}

func (r *Reconciler) drop(app string) {
	r.mu.Lock()
	delete(r.windows, app)
	r.mu.Unlock()
}

func (r *Reconciler) windowFor(rel store.CanaryRelease, pair Pair, totals map[string]telemetry.UpstreamStat) *window {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.windows[rel.ServiceName]
	if w == nil || w.image != rel.Image || w.stage != rel.StageIndex || w.pair != pair {
		w = &window{
			image: rel.Image, stage: rel.StageIndex, pair: pair, start: r.now(),
			canary: SampleFrom(totals[pair.Canary]), stable: SampleFrom(totals[pair.Stable]),
		}
		r.windows[rel.ServiceName] = w
	}
	return w
}

func (r *Reconciler) input(rel store.CanaryRelease, w *window, totals map[string]telemetry.UpstreamStat) Input {
	return Input{
		Policy: policyOf(rel), Limits: r.limits, Stage: rel.StageIndex, Elapsed: r.now().Sub(w.start),
		Canary:   SampleFrom(totals[w.pair.Canary]).Sub(w.canary),
		Baseline: SampleFrom(totals[w.pair.Stable]).Sub(w.stable),
	}
}

// Progress is the live view of a staged canary for the dashboard, CLI and MCP.
type Progress struct {
	Stage          int     `json:"stage"`
	StageCount     int     `json:"stage_count"`
	ElapsedSecs    int     `json:"elapsed_secs"`
	IntervalSecs   int     `json:"interval_secs"`
	CanaryRequests uint64  `json:"canary_requests"`
	CanaryErrRate  float64 `json:"canary_error_rate"`
	CanaryP95MS    float64 `json:"canary_p95_ms"`
	StableRequests uint64  `json:"stable_requests"`
	StableErrRate  float64 `json:"stable_error_rate"`
	StableP95MS    float64 `json:"stable_p95_ms"`
	Next           string  `json:"next"`
	Breaching      bool    `json:"breaching"`
}

// Progress returns the current observation of rel, false when it is not being
// observed (not staged, paused, or not yet receiving traffic).
func (r *Reconciler) Progress(rel store.CanaryRelease) (Progress, bool) {
	if !rel.Staged() {
		return Progress{}, false
	}
	r.mu.Lock()
	w := r.windows[rel.ServiceName]
	r.mu.Unlock()
	if w == nil || w.stage != rel.StageIndex || w.image != rel.Image {
		return Progress{}, false
	}
	in := r.input(rel, w, r.stats.UpstreamTotals())
	d := Decide(in)
	return Progress{
		Stage: rel.StageIndex + 1, StageCount: len(rel.Stages),
		ElapsedSecs: int(in.Elapsed / time.Second), IntervalSecs: rel.IntervalSecs,
		CanaryRequests: in.Canary.Requests, CanaryErrRate: in.Canary.ErrorRate(), CanaryP95MS: in.Canary.P95(),
		StableRequests: in.Baseline.Requests, StableErrRate: in.Baseline.ErrorRate(), StableP95MS: in.Baseline.P95(),
		Next: d.Reason, Breaching: d.Action == Rollback,
	}, true
}

var defaultDials = NewDials()

// DefaultDials is the process-wide record the ingress controller writes and
// the reconciler reads, mirroring ingress.DefaultRequestStats.
func DefaultDials() *Dials { return defaultDials }
