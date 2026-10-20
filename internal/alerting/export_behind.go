package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExportHealth is one export destination's delivery state, as the engine sees it.
type ExportHealth struct {
	Name          string
	Enabled       bool
	CreatedAt     time.Time
	LastSuccessAt time.Time
	LastError     string
	CircuitOpen   bool
}

// ExportHealthSource lists every export destination's delivery state.
type ExportHealthSource interface {
	ExportHealth(ctx context.Context) ([]ExportHealth, error)
}

// DefaultExportBehindAfter is how long a failing destination may go without a
// successful delivery before the rule fires.
const DefaultExportBehindAfter = 15 * time.Minute

// EvaluateExportBehind runs one KindExportBehind rule. A destination is
// behind when it is failing (or its breaker is open) and has not delivered
// within the rule's ForDuration (else DefaultExportBehindAfter).
func EvaluateExportBehind(ctx context.Context, source ExportHealthSource, r Rule, now time.Time) (Rule, string, error) {
	dests, err := source.ExportHealth(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read export health: %w", r.ID, err)
	}
	limit := r.ForDuration
	if limit <= 0 {
		limit = DefaultExportBehindAfter
	}
	var bad []string
	for _, d := range dests {
		if !d.Enabled || (d.LastError == "" && !d.CircuitOpen) {
			continue
		}
		ref := d.LastSuccessAt
		if ref.IsZero() {
			ref = d.CreatedAt
		}
		if age := now.Sub(ref); age > limit {
			reason := d.LastError
			if reason == "" {
				reason = "circuit open"
			}
			bad = append(bad, fmt.Sprintf("%s: no delivery for %s (%s)", d.Name, age.Round(time.Minute), truncateNotice(reason, 120)))
		}
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "export destination behind: " + strings.Join(bad, "; ")
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}
