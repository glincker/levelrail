package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LogArchiveHealth is one log archive policy's freshness, as the engine sees it.
type LogArchiveHealth struct {
	// Scope is the app name, or empty for the global policy.
	Scope         string
	Enabled       bool
	Interval      time.Duration
	CreatedAt     time.Time
	LastSuccessAt time.Time
	LastError     string
}

// LogArchiveSource lists every log archive policy's health.
type LogArchiveSource interface {
	LogArchiveHealth(ctx context.Context) ([]LogArchiveHealth, error)
}

// minLogArchiveMaxAge keeps a short-interval policy from flapping on one late run.
const minLogArchiveMaxAge = 2 * time.Hour

// EvaluateLogArchiveStale runs one KindLogArchiveStale rule. A policy is
// unhealthy when its last run failed or it has not succeeded within the
// rule's ForDuration (else three intervals, at least two hours).
func EvaluateLogArchiveStale(ctx context.Context, source LogArchiveSource, r Rule, now time.Time) (Rule, string, error) {
	policies, err := source.LogArchiveHealth(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read log archive health: %w", r.ID, err)
	}

	var bad []string
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if reason := logArchiveProblem(p, r.ForDuration, now); reason != "" {
			bad = append(bad, fmt.Sprintf("%s: %s", logArchiveScopeName(p.Scope), reason))
		}
	}

	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(bad))
	next.LastValue = &v

	var notice string
	if len(bad) > 0 {
		notice = "log archive unhealthy: " + strings.Join(bad, "; ")
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}

func logArchiveScopeName(scope string) string {
	if scope == "" {
		return "all apps"
	}
	return scope
}

func logArchiveProblem(p LogArchiveHealth, override time.Duration, now time.Time) string {
	if p.LastError != "" {
		return "last run failed (" + truncateNotice(p.LastError, 160) + ")"
	}
	maxAge := override
	if maxAge <= 0 {
		maxAge = max(3*p.Interval, minLogArchiveMaxAge)
	}
	ref := p.LastSuccessAt
	if ref.IsZero() {
		ref = p.CreatedAt
	}
	if age := now.Sub(ref); age > maxAge {
		return fmt.Sprintf("no successful archive for %s (limit %s)", age.Round(time.Minute), maxAge)
	}
	return ""
}

func truncateNotice(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
