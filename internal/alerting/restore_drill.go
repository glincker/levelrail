package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RestoreDrillProblem is one resource whose latest restore drill failed.
type RestoreDrillProblem struct {
	Resource string
	Reason   string
	At       time.Time
}

// RestoreDrillSource lists resources whose most recent restore drill failed.
type RestoreDrillSource interface {
	RestoreDrillProblems(ctx context.Context) ([]RestoreDrillProblem, error)
}

// EvaluateRestoreDrillFailed runs one KindRestoreDrillFailed rule. It fires
// while any resource's latest restore drill is failed and resolves once a
// later drill passes.
func EvaluateRestoreDrillFailed(ctx context.Context, source RestoreDrillSource, r Rule, now time.Time) (Rule, string, error) {
	problems, err := source.RestoreDrillProblems(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read restore drills: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(problems))
	next.LastValue = &v

	var notice string
	if len(problems) > 0 {
		parts := make([]string, 0, len(problems))
		for _, p := range problems {
			parts = append(parts, p.Resource+": "+truncateNotice(p.Reason, 160))
		}
		notice = "restore drill failed: " + strings.Join(parts, "; ")
	}
	return advanceState(next, r, len(problems) > 0, 0, now), notice, nil
}
