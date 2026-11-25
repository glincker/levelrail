package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LaggingReconciler is one reconciler kind over the lag warning.
type LaggingReconciler struct {
	Reconciler string
	Lag        time.Duration
}

// ReconcilerLagSource lists the reconcilers currently over the lag warning.
type ReconcilerLagSource interface {
	LaggingReconcilers(ctx context.Context) ([]LaggingReconciler, error)
}

// EvaluateReconcilerLagging runs one KindReconcilerLagging rule. It fires while
// any reconciler is lagging and resolves once every one has converged.
func EvaluateReconcilerLagging(ctx context.Context, source ReconcilerLagSource, r Rule, now time.Time) (Rule, string, error) {
	lagging, err := source.LaggingReconcilers(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read reconciler lag: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(lagging))
	next.LastValue = &v
	var notice string
	if len(lagging) > 0 {
		parts := make([]string, 0, len(lagging))
		for _, l := range lagging {
			parts = append(parts, fmt.Sprintf("%s: %s", l.Reconciler, l.Lag.Round(time.Second)))
		}
		notice = "reconciler lagging: " + strings.Join(parts, "; ")
	}
	return advanceState(next, r, len(lagging) > 0, 0, now), notice, nil
}
