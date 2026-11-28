package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/telemetry"
)

// Seeded rule IDs whose thresholds follow live runtime settings instead of the stored copy.
const (
	DefaultCrashloopRuleID = defaultRulePrefix + "crashloop"
	DefaultErrorRateRuleID = defaultRulePrefix + "error-rate-5xx"
)

// DefaultThresholds are the live values behind the seeded crash loop and 5xx rules.
type DefaultThresholds struct {
	CrashloopRestarts int
	ErrorRatePercent  float64
	// MinRequests keeps one failed request on a quiet app from reading as 100 percent.
	MinRequests float64
}

// IsDefaultRule reports whether r is one of the seeded platform rules.
func IsDefaultRule(r Rule) bool {
	return r.ResourceID == DefaultRuleResource && strings.HasPrefix(r.ID, defaultRulePrefix)
}

// WorstSince returns the resource with the most restarts after since; count is 0 when none restarted.
func (t *RestartTracker) WorstSince(since time.Time) (resourceID string, count int) {
	if t == nil {
		return "", 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, stamps := range t.restarts {
		n := 0
		for _, ts := range stamps {
			if ts.After(since) {
				n++
			}
		}
		if n > count || (n == count && n > 0 && id < resourceID) {
			resourceID, count = id, n
		}
	}
	return resourceID, count
}

// EvaluateCrashloopAll is the platform-wide crash loop rule: it fires while any app restarts threshold times inside the window.
func EvaluateCrashloopAll(tracker *RestartTracker, r Rule, threshold int, now time.Time) (Rule, string) {
	if threshold > 0 {
		r.RestartCountThreshold = threshold
	}
	worst, count := tracker.WorstSince(now.Add(-r.RestartWindow))
	next := r
	next.LastEvaluatedAt = &now
	v := float64(count)
	next.LastValue = &v
	notice := ""
	if count >= r.RestartCountThreshold {
		notice = fmt.Sprintf("%s restarted %d times in %s", strings.TrimPrefix(worst, "service:"), count, r.RestartWindow)
	}
	return advanceState(next, r, count >= r.RestartCountThreshold, 0, now), notice
}

// EvaluateErrorRateAll is the platform-wide 5xx rule: it fires while any app's 5xx share over the request window meets the threshold.
func EvaluateErrorRateAll(ctx context.Context, source MetricsSource, apps []string, r Rule, th DefaultThresholds, now time.Time) (Rule, string, error) {
	if th.ErrorRatePercent > 0 {
		r.Threshold = th.ErrorRatePercent
	}
	window := requestSignalWindow()
	next := r
	next.LastEvaluatedAt = &now
	var worstApp string
	var worst float64
	var firstErr error
	for _, app := range apps {
		points, err := telemetry.QueryRequests(ctx, source, app, now.Add(-window), now, window+time.Second)
		if err != nil && len(points) == 0 {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(points) == 0 || points[0].Requests < th.MinRequests {
			continue
		}
		if pct := points[0].ErrorRate5xx * 100; pct > worst {
			worst, worstApp = pct, app
		}
	}
	v := worst
	next.LastValue = &v
	notice := ""
	hit := worstApp != "" && worst >= r.Threshold
	if hit {
		notice = fmt.Sprintf("%s: %.1f%% of responses were 5xx over the last %s", worstApp, worst, window)
	}
	return advanceState(next, r, hit, r.ForDuration, now), notice, firstErr
}
