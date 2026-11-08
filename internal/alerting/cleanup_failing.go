package alerting

import (
	"context"
	"fmt"
	"time"
)

// CleanupFailures is what the cleanup center reports for one evaluation.
// Window and DefaultFloor come from runtime settings, resolved by the source.
type CleanupFailures struct {
	Failed       int
	Window       time.Duration
	DefaultFloor int
	LastTarget   string
}

// CleanupSource reads failed cleanup runs since now minus the configured window.
type CleanupSource interface {
	CleanupFailures(ctx context.Context, now time.Time) (CleanupFailures, error)
}

// EvaluateCleanupFailing fires while failed real cleanup runs in the window reach the rule Threshold, or the settings default when it is 0.
func EvaluateCleanupFailing(ctx context.Context, src CleanupSource, r Rule, now time.Time) (Rule, string, error) {
	st, err := src.CleanupFailures(ctx, now)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: cleanup failures: %w", r.ID, err)
	}
	floor := int(r.Threshold)
	if floor <= 0 {
		floor = st.DefaultFloor
	}
	if floor <= 0 {
		floor = 1
	}
	next := r
	next.LastEvaluatedAt = &now
	failing := st.Failed >= floor
	var notice string
	if failing {
		notice = fmt.Sprintf("%d cleanup runs failed in the last %d minutes", st.Failed, int(st.Window.Minutes()))
		if st.LastTarget != "" {
			notice += " (latest: " + st.LastTarget + ")"
		}
	}
	return advanceState(next, r, failing, r.ForDuration, now), notice, nil
}
