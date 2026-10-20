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

// EvaluateThreshold returns the rule's updated state and never writes to the database itself.
// The condition must hold for ForDuration before firing, but one good sample resolves it at once, on purpose: a false resolve is less harmful than a false page.
func EvaluateThreshold(ctx context.Context, source MetricsSource, r Rule, now time.Time) (Rule, error) {
	// The short lookback finds only the latest sample; ForDuration is tracked across ticks via PendingSince, not by scanning its range.
	const lookback = 60 * time.Second

	var samples []telemetry.Sample
	if IsRequestSignal(r.Metric) {
		v, ok, err := requestSignalValue(ctx, source, r, now)
		if err != nil {
			return r, fmt.Errorf("alerting: evaluate rule %q: request signal: %w", r.ID, err)
		}
		if ok {
			samples = []telemetry.Sample{{ResourceID: r.ResourceID, Metric: r.Metric, Timestamp: now, Value: v}}
		}
	} else {
		var err error
		samples, err = source.QueryMetrics(ctx, r.ResourceID, r.Metric, now.Add(-lookback), now)
		if err != nil {
			return r, fmt.Errorf("alerting: evaluate rule %q: query metrics: %w", r.ID, err)
		}
	}

	next := r
	next.LastEvaluatedAt = &now

	if len(samples) == 0 {
		// No recent data counts as not satisfied, so a silent service does not keep a firing alert alive. LastValue is kept so the last reading stays visible.
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, nil
	}

	latest := samples[len(samples)-1]
	next.LastValue = &latest.Value

	return advanceState(next, r, satisfies(latest.Value, r.Comparator, r.Threshold), r.ForDuration, now), nil
}

// advanceState is the pending/firing state machine shared by EvaluateThreshold and EvaluateCrashloop.
// Callers set LastEvaluatedAt and LastValue before calling it.
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
