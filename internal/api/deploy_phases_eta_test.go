package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMedianMS(t *testing.T) {
	cases := []struct {
		name string
		in   []int64
		want int64
	}{
		{"empty", nil, 0},
		{"single", []int64{7}, 7},
		{"odd", []int64{9, 1, 5}, 5},
		{"even", []int64{4, 2, 8, 6}, 5},
		{"outlier ignored", []int64{100, 110, 90, 105, 9000}, 105},
	}
	for _, c := range cases {
		if got := medianMS(c.in); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

type etaFakeAttempts struct {
	DeployAttemptStore
	list []store.DeployAttempt
}

func (f etaFakeAttempts) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return f.list, nil
}

func (f etaFakeAttempts) ListDeployTraceSteps(_ context.Context, id string) ([]store.DeployTraceStep, error) {
	var t0 time.Time
	for _, a := range f.list {
		if a.ID == id {
			t0 = a.StartedAt
		}
	}
	return []store.DeployTraceStep{
		{Step: deploytrace.StepContainerStart, Status: store.TraceStatusDone, StartedAt: t0, EndedAt: t0.Add(40 * time.Second)},
		{Step: deploytrace.StepCutover, Status: store.TraceStatusDone, StartedAt: t0.Add(50 * time.Second), EndedAt: t0.Add(60 * time.Second)},
	}, nil
}

type etaFakePerf struct {
	BuildPerformanceStore
	list []store.DeployAttempt
}

func (f etaFakePerf) ListBuildAttempts(context.Context, int) ([]store.DeployAttempt, error) {
	return f.list, nil
}

func etaHistory(prefix string, n int) []store.DeployAttempt {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]store.DeployAttempt, 0, n)
	for i := range n {
		st := t0.Add(time.Duration(i) * time.Hour)
		fin := st.Add(time.Minute)
		out = append(out, store.DeployAttempt{
			ID: prefix + string(rune('a'+i)), ServiceName: "web", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusSucceeded, StartedAt: st, FinishedAt: &fin,
		})
	}
	return out
}

func etaRunning(elapsed time.Duration) (store.DeployAttempt, *deployPhasesResponse) {
	a := store.DeployAttempt{ID: "run", ServiceName: "web", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now().Add(-elapsed)}
	resp := &deployPhasesResponse{ElapsedMS: elapsed.Milliseconds(), Phases: derivePhases(a, nil, nil, time.Now())}
	return a, resp
}

func TestApplyETAAppHistory(t *testing.T) {
	rt := &Router{deployAttempts: etaFakeAttempts{list: etaHistory("a", 3)}}
	a, resp := etaRunning(20 * time.Second)
	rt.applyETA(context.Background(), a, resp)
	if resp.ETABasis != "app_history" || resp.Samples != 3 || resp.ETASeconds == nil || *resp.ETASeconds != 50 {
		t.Fatalf("unexpected eta: basis=%s samples=%d secs=%v", resp.ETABasis, resp.Samples, resp.ETASeconds)
	}
	raw, _ := json.Marshal(resp)
	for _, key := range []string{`"eta_seconds":50`, `"eta_basis":"app_history"`, `"eta_ms":50000`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("json missing %s: %s", key, raw)
		}
	}
}

func TestApplyETAPlatformFallbackAndNone(t *testing.T) {
	// One own sample is below the minimum, so the platform history is used.
	rt := &Router{deployAttempts: etaFakeAttempts{list: etaHistory("a", 1)}, buildPerf: etaFakePerf{list: etaHistory("p", 4)}}
	a, resp := etaRunning(10 * time.Second)
	rt.applyETA(context.Background(), a, resp)
	if resp.ETABasis != "platform_history" || resp.Samples != 4 || resp.ETASeconds == nil {
		t.Fatalf("expected platform basis: %+v", resp)
	}

	rt = &Router{deployAttempts: etaFakeAttempts{list: etaHistory("a", 1)}}
	a, resp = etaRunning(10 * time.Second)
	rt.applyETA(context.Background(), a, resp)
	if resp.ETABasis != "none" || resp.ETASeconds != nil || resp.ETAMS != nil {
		t.Fatalf("expected no estimate: %+v", resp)
	}
}
