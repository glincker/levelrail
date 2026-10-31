package analysis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrBusy means every report slot is in use; callers answer 429 and retry later.
var ErrBusy = errors.New("analysis: report capacity is full")

// Saver persists reports; *store.DB satisfies it.
type Saver interface {
	SaveAnalysisReport(ctx context.Context, r store.AnalysisReport) error
	PruneAnalysisReports(ctx context.Context, cutoff time.Time) (int64, error)
}

// Runner generates reports off the alert path. A nil Runner is a no-op,
// which is what a disabled feature gets.
type Runner struct {
	cfg    Config
	src    Sources
	saver  Saver
	logger *slog.Logger
	now    func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
	sem  chan struct{}
	wg   sync.WaitGroup
}

// NewRunner returns nil unless cfg.Enabled, so callers can wire it unconditionally.
func NewRunner(cfg Config, src Sources, saver Saver, logger *slog.Logger) *Runner {
	if !cfg.Enabled || saver == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{cfg: cfg, src: src, saver: saver, logger: logger, now: time.Now, last: map[string]time.Time{}, sem: make(chan struct{}, max(cfg.MaxConcurrent, 1))}
}

// OnAlert is the alerting hook. It only filters and schedules; any failure
// is logged and never reaches the alert pipeline.
func (r *Runner) OnAlert(e alerting.HistoryEntry) {
	if r == nil || e.Event != alerting.EventFired || e.App == "" || e.Outcome == alerting.OutcomeSkipped || e.Outcome == alerting.OutcomeQuiet {
		return
	}
	kind := TriggerAlert
	if e.RuleKind == string(alerting.KindBudgetThreshold) {
		kind = TriggerSpend
	}
	tr := Trigger{Kind: kind, ID: e.ID, App: e.App, RuleName: e.RuleName, Severity: e.Severity, Detail: e.Detail, At: e.At}
	if !r.admit(e.App) {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() { <-r.sem }()
		defer func() {
			if p := recover(); p != nil {
				r.logger.Error("analysis: report generation panicked", slog.Any("panic", p))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
		defer cancel()
		if _, err := r.Generate(ctx, tr); err != nil {
			r.logger.Warn("analysis: report generation failed", slog.String("app", tr.App), slog.String("error", err.Error()))
		}
	}()
}

// Wait blocks until in-flight reports finish; tests use it.
func (r *Runner) Wait() { r.wg.Wait() }

// admit applies the per-app interval and the concurrency cap; on true the caller owns a sem slot.
func (r *Runner) admit(app string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if t, ok := r.last[app]; ok && now.Sub(t) < r.cfg.MinInterval {
		return false
	}
	select {
	case r.sem <- struct{}{}:
		r.last[app] = now
		return true
	default:
		return false
	}
}

// GenerateManual runs one operator-requested report inside the same concurrency cap and
// timeout as alert-driven ones, without the per-app interval.
func (r *Runner) GenerateManual(ctx context.Context, tr Trigger) (store.AnalysisReport, error) {
	select {
	case r.sem <- struct{}{}:
	default:
		return store.AnalysisReport{}, ErrBusy
	}
	defer func() { <-r.sem }()
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	return r.Generate(ctx, tr)
}

// Generate collects, builds and stores one report.
func (r *Runner) Generate(ctx context.Context, tr Trigger) (store.AnalysisReport, error) {
	now := r.now()
	end := tr.At
	if end.IsZero() || end.After(now) {
		end = now
	}
	rep := Build(tr, Collect(ctx, r.src, r.cfg, tr.App, end), r.cfg, now)
	body, err := json.Marshal(rep)
	if err != nil {
		return store.AnalysisReport{}, fmt.Errorf("analysis: marshal report: %w", err)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return store.AnalysisReport{}, fmt.Errorf("analysis: report id: %w", err)
	}
	row := store.AnalysisReport{ID: "ar_" + hex.EncodeToString(raw[:]), App: tr.App, TriggerKind: tr.Kind, TriggerID: tr.ID, Status: rep.Status, SchemaVersion: SchemaVersion, Report: body, CreatedAt: now}
	if err := r.saver.SaveAnalysisReport(ctx, row); err != nil {
		return store.AnalysisReport{}, err
	}
	if _, err := r.saver.PruneAnalysisReports(ctx, now.Add(-r.cfg.Retention)); err != nil {
		r.logger.Warn("analysis: prune reports failed", slog.String("error", err.Error()))
	}
	return row, nil
}
