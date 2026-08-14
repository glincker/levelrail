package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// MetricsSource is the narrow surface a threshold evaluation needs.
// *telemetry.Federator satisfies this structurally; a hand-written fake
// in tests avoids needing a real store for pure evaluation-logic tests.
type MetricsSource interface {
	QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error)
}

// EvaluateThreshold runs one KindThreshold rule against source and
// returns its updated evaluation state, to be persisted via
// DB.UpdateState by the caller (engine.go). Pure with respect to
// persistence: this function never writes to the database itself, so
// its state-machine logic (the part actually worth testing precisely)
// is testable without a real DB.
//
// Semantics: the rule's condition must hold continuously, checked on
// every evaluation tick, for at least ForDuration before it graduates
// from "pending" to "firing" (avoiding a single noisy sample flapping
// a brand-new alert into existence). Once firing, it stays firing until
// a tick observes the condition no longer holding, at which point it
// resets to not-pending/not-firing immediately: there's no separate
// "resolving" debounce, a single good sample is enough to consider it
// recovered, asymmetric on purpose since false-positive resolution
// (briefly saying "recovered" when it isn't) is a much smaller problem
// than false-positive firing (paging someone for a blip).
func EvaluateThreshold(ctx context.Context, source MetricsSource, r Rule, now time.Time) (Rule, error) {
	// A short lookback (twice the collection interval Collector.Run
	// uses in cmd/levelrail/main.go would be the ideal value to share,
	// but this package doesn't depend on cmd/levelrail; 60s comfortably
	// covers the 15s collection tick with margin for a slow write) is
	// enough to find "the latest sample," not the whole evaluation
	// window: ForDuration governs how long the condition must have been
	// true, tracked via PendingSince across ticks, not by requesting a
	// ForDuration-wide range and inspecting every sample in it.
	const lookback = 60 * time.Second

	samples, err := source.QueryMetrics(ctx, r.ResourceID, r.Metric, now.Add(-lookback), now)
	if err != nil {
		return r, fmt.Errorf("alerting: evaluate rule %q: query metrics: %w", r.ID, err)
	}

	next := r
	next.LastEvaluatedAt = &now

	if len(samples) == 0 {
		// No recent data: neither confirms nor denies the condition.
		// Treat like "condition not satisfied" for pending/firing
		// purposes (a service that stopped reporting shouldn't keep an
		// old firing alert artificially alive forever), but leave
		// LastValue as whatever it last was rather than clearing it, so
		// an operator can still see the last real reading.
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, nil
	}

	latest := samples[len(samples)-1]
	next.LastValue = &latest.Value

	return advanceState(next, r, satisfies(latest.Value, r.Comparator, r.Threshold), r.ForDuration, now), nil
}

// advanceState is the pending/firing state machine shared by
// EvaluateThreshold and EvaluateCrashloop (crashloop.go): both reduce to
// "is the condition satisfied on this tick," then apply the identical
// debounce-in, clear-immediately-out logic documented on
// EvaluateThreshold. next already has LastEvaluatedAt/LastValue set by
// the caller; this only touches Pending/Firing/FiringSince.
func advanceState(next, prev Rule, satisfied bool, forDuration time.Duration, now time.Time) Rule {
	if !satisfied {
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next
	}

	switch {
	case prev.Firing:
		next.PendingSince = prev.PendingSince
		next.Firing = true
		next.FiringSince = prev.FiringSince
	case prev.PendingSince != nil:
		if now.Sub(*prev.PendingSince) >= forDuration {
			next.Firing = true
			since := *prev.PendingSince
			next.FiringSince = &since
		} else {
			next.PendingSince = prev.PendingSince
		}
	default:
		if forDuration <= 0 {
			next.Firing = true
			next.FiringSince = &now
		} else {
			next.PendingSince = &now
		}
	}

	return next
}

func satisfies(value float64, c Comparator, threshold float64) bool {
	switch c {
	case GreaterThan:
		return value > threshold
	case LessThan:
		return value < threshold
	case GreaterOrEqual:
		return value >= threshold
	case LessOrEqual:
		return value <= threshold
	default:
		slog.Default().Warn("alerting: unknown comparator, treating as never-satisfied", slog.String("comparator", string(c)))
		return false
	}
}
