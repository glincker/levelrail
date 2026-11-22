package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeDurations []DurationSeries

func (f fakeDurations) RecentDurations(context.Context) ([]DurationSeries, error) { return f, nil }

func TestEvaluateDeployDurationRegression(t *testing.T) {
	now := time.Now()
	r := Rule{ID: "r", Kind: KindDeployDurationRegression}
	pol := DurationRegressionPolicy{Percent: 50, Recent: 2, Baseline: 3}
	src := fakeDurations{
		{App: "web", Stage: DurationStageBuild, Seconds: []int64{90, 95, 60, 62, 58}},
		{App: "web", Stage: DurationStageDeploy, Seconds: []int64{30, 31, 30, 29, 32}},
		{App: "young", Stage: DurationStageBuild, Seconds: []int64{900, 10}},
	}
	next, notice, err := EvaluateDeployDurationRegression(context.Background(), src, pol, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "web build") || strings.Contains(notice, "web deploy") || strings.Contains(notice, "young") {
		t.Fatalf("notice = %q", notice)
	}
	if next.LastValue == nil || *next.LastValue != 1 {
		t.Fatalf("last value = %v", next.LastValue)
	}
	if _, notice, _ = EvaluateDeployDurationRegression(context.Background(), src, DurationRegressionPolicy{}, r, now); notice != "" {
		t.Fatalf("disabled policy fired: %q", notice)
	}
}
