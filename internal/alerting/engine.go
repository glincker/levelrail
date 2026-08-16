package alerting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// LogsSource is the narrow surface Engine needs to attach log lines to
// a firing crashloop event. *telemetry.Federator satisfies this
// structurally, the same as MetricsSource in evaluate.go.
type LogsSource interface {
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
}

// RuleStore is the narrow store surface Engine needs: list what to
// evaluate, persist the result. *DB satisfies this structurally.
type RuleStore interface {
	ListEnabledRules(ctx context.Context) ([]Rule, error)
	UpdateState(ctx context.Context, id string, pendingSince, firingSince *time.Time, firing bool, evaluatedAt time.Time, value *float64) error
}

// crashloopLogLines is TASKS.md 2.7's literal number: "the last 200
// lines."
const crashloopLogLines = 200

// crashloopLogLookback bounds how far back Engine searches for those
// 200 lines. A crashlooping container's logs from the last 15 minutes
// are what's actually useful to see; going back further risks pulling
// in an earlier, unrelated incident's output instead.
const crashloopLogLookback = 15 * time.Minute

// Engine evaluates every enabled rule on an interval, persists each
// rule's updated state, and notifies only on a firing/resolved
// transition, never on every tick a rule stays in the same state
// (the same reasoning already documented on Event.Resolved:
// repeated identical notifications train an operator to ignore the
// channel).
type Engine struct {
	rules       RuleStore
	metrics     MetricsSource
	logs        LogsSource
	tracker     *RestartTracker
	newNotifier func(Rule) Notifier
	logger      *slog.Logger
}

// NewEngine builds an Engine. newNotifier defaults to alerting.NewNotifier
// with the standard http.DefaultClient-backed Notifier and no email
// capability configured (an email-kind rule then always fails with a
// clear "not configured" error) if nil. A caller that does have email
// configured (cmd/levelrail/main.go, via internal/email) passes its own
// closure capturing a real email.Sender instead; tests override it the
// same way to capture Notify calls without a real HTTP server.
func NewEngine(rules RuleStore, metrics MetricsSource, logs LogsSource, tracker *RestartTracker, newNotifier func(Rule) Notifier, logger *slog.Logger) *Engine {
	if newNotifier == nil {
		newNotifier = func(r Rule) Notifier { return NewNotifier(nil, nil, r) }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{rules: rules, metrics: metrics, logs: logs, tracker: tracker, newNotifier: newNotifier, logger: logger}
}

// Tick evaluates every enabled rule once. Errors from individual rules
// (a metrics query failing, a notification failing to send) are
// collected and joined, never stopping evaluation of the remaining
// rules: the same "one broken resource must not block the rest"
// principle reconcile.Engine.ReconcileAll already applies to
// controllers, applied here to alert rules.
func (e *Engine) Tick(ctx context.Context) error {
	rules, err := e.rules.ListEnabledRules(ctx)
	if err != nil {
		return fmt.Errorf("alerting: list enabled rules: %w", err)
	}

	now := time.Now()
	var errs []error

	for _, r := range rules {
		var next Rule
		switch r.Kind {
		case KindThreshold:
			next, err = EvaluateThreshold(ctx, e.metrics, r, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %q: %w", r.ID, err))
				continue
			}
		case KindCrashloop:
			next = EvaluateCrashloop(e.tracker, r, now)
		default:
			e.logger.Warn("alerting: rule has unknown kind, skipping", slog.String("rule_id", r.ID), slog.String("kind", string(r.Kind)))
			continue
		}

		becameFiring := next.Firing && !r.Firing
		becameResolved := !next.Firing && r.Firing

		if err := e.rules.UpdateState(ctx, r.ID, next.PendingSince, next.FiringSince, next.Firing, now, next.LastValue); err != nil {
			errs = append(errs, fmt.Errorf("rule %q: persist state: %w", r.ID, err))
			continue
		}

		switch {
		case becameFiring:
			e.dispatch(ctx, next, false)
		case becameResolved:
			e.dispatch(ctx, next, true)
		}
	}

	return errors.Join(errs...)
}

// dispatch sends one notification for r's transition. Failures are
// logged, not returned: a notification failing to send must never stop
// evaluation of other rules, and the rule's own state was already
// persisted successfully before dispatch is called, so a lost
// notification doesn't leave the rule's stored state inconsistent with
// reality, only the operator momentarily uninformed.
func (e *Engine) dispatch(ctx context.Context, r Rule, resolved bool) {
	ev := Event{Rule: r, Resolved: resolved}
	if r.Kind == KindCrashloop && !resolved {
		ev.LogLines = e.fetchRecentLogLines(ctx, r.ResourceID)
	}

	if err := e.newNotifier(r).Notify(ctx, ev); err != nil {
		e.logger.Error("alerting: notification failed",
			slog.String("rule_id", r.ID), slog.String("resource_id", r.ResourceID),
			slog.Bool("resolved", resolved), slog.String("error", err.Error()))
	}
}

// fetchRecentLogLines returns the most recent up to crashloopLogLines
// messages for resourceID, oldest of that tail first (matching how a
// log viewer reads top-to-bottom). A query failure returns nil, logged,
// not propagated: a firing crashloop notification without log lines
// attached is still far more useful than no notification at all.
func (e *Engine) fetchRecentLogLines(ctx context.Context, resourceID string) []string {
	now := time.Now()
	entries, err := e.logs.QueryLogs(ctx, resourceID, now.Add(-crashloopLogLookback), now, "")
	if err != nil {
		e.logger.Error("alerting: fetch crashloop log lines failed",
			slog.String("resource_id", resourceID), slog.String("error", err.Error()))
		return nil
	}

	if len(entries) > crashloopLogLines {
		entries = entries[len(entries)-crashloopLogLines:]
	}
	lines := make([]string, len(entries))
	for i, entry := range entries {
		lines[i] = entry.Message
	}
	return lines
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase
// (telemetry.Collector.Run, cmd/levelrail's retention sweeps).
func (e *Engine) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := e.Tick(ctx); err != nil {
				e.logger.Warn("alerting: evaluation tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}
