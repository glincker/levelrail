package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// KindDeployDurationRegression is platform-wide: it fires while an app's build or deploy p50 is
// well above its own earlier p50. Each notice line names the stage that grew.
const KindDeployDurationRegression Kind = "deploy_duration_regression"

// Stages a DurationSeries can describe.
const (
	DurationStageBuild  = "build"
	DurationStageDeploy = "deploy"
)

// DurationSeries holds one app's succeeded run durations in seconds for one stage, newest first.
type DurationSeries struct {
	App     string
	Stage   string
	Seconds []int64
}

// DurationSource lists the recent succeeded build and deploy durations per app.
type DurationSource interface {
	RecentDurations(ctx context.Context) ([]DurationSeries, error)
}

// DurationRegressionPolicy is the live setting trio: Percent 0 turns the check off, Recent is how
// many newest runs form the current p50 and Baseline how many runs before them form the baseline p50.
type DurationRegressionPolicy struct {
	Percent  int
	Recent   int
	Baseline int
}

// EvaluateDeployDurationRegression runs one KindDeployDurationRegression rule. A series needs
// Recent+Baseline runs before it is judged, so a young app never fires.
func EvaluateDeployDurationRegression(ctx context.Context, source DurationSource, policy DurationRegressionPolicy, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	if policy.Percent <= 0 || policy.Recent <= 0 || policy.Baseline <= 0 {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}
	series, err := source.RecentDurations(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read durations: %w", r.ID, err)
	}
	var bad []string
	for _, s := range series {
		if len(s.Seconds) < policy.Recent+policy.Baseline {
			continue
		}
		cur := medianInt64(s.Seconds[:policy.Recent])
		base := medianInt64(s.Seconds[policy.Recent : policy.Recent+policy.Baseline])
		if base <= 0 || cur*100 <= base*int64(100+policy.Percent) {
			continue
		}
		bad = append(bad, fmt.Sprintf("%s %s: p50 is %ds, up %d%% from a usual %ds", s.App, s.Stage, cur, (cur-base)*100/base, base))
	}
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "deploy duration regression: " + strings.Join(bad, "; ")
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}
