package deploytrace

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type stageSink struct {
	mu   sync.Mutex
	rows map[string]store.DeployTraceStep
}

func (m *stageSink) SaveDeployTraceStep(_ context.Context, s store.DeployTraceStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]store.DeployTraceStep{}
	}
	m.rows[s.Key] = s
	return nil
}

func (m *stageSink) list() []store.DeployTraceStep {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.DeployTraceStep
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out
}

func TestClassifyStage(t *testing.T) {
	cases := map[string]string{
		"[internal] load build definition from Dockerfile": StageSource,
		"builder":                 StageDetect,
		"[3/6] RUN npm ci":        StageInstall,
		"[5/6] RUN npm run build": StageBuild,
		"exporting to image":      StageExport,
		"load image":              StagePush,
		"":                        "",
	}
	for in, want := range cases {
		if got := ClassifyStage(in); got != want {
			t.Errorf("ClassifyStage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStageTrackerRecordsStagesCacheAndTail(t *testing.T) {
	sink := &stageSink{}
	tr := New(sink, "att1", nil)
	tracker := NewStageTracker(tr, func() int { return 2 })
	ctx := context.Background()

	tracker.Observe(ctx, StageObs{Step: "[internal] load build context", Completed: true, Cached: true})
	tracker.Observe(ctx, StageObs{Step: "[3/5] RUN npm ci", Completed: true, Cached: true})
	tracker.Observe(ctx, StageObs{Step: "[3/5] RUN npm ci", Log: "a\nb\nc\n"})
	if got := tracker.Observe(ctx, StageObs{Log: "bare line\n"}); got != StageInstall {
		t.Fatalf("bare line stage = %q, want install", got)
	}
	tracker.Observe(ctx, StageObs{Step: "[4/5] RUN npm run build", Completed: true})
	tracker.Observe(ctx, StageObs{Step: "exporting to image", Completed: true})
	tracker.Finish(ctx)

	views := StagesFromRows(sink.list())
	var order []string
	byStage := map[string]StageView{}
	for _, v := range views {
		order = append(order, v.Stage)
		byStage[v.Stage] = v
	}
	if strings.Join(order, ",") != "source,install,build,export" {
		t.Fatalf("stage order = %v", order)
	}
	if byStage[StageSource].Cache != CacheHit || byStage[StageBuild].Cache != CacheMiss {
		t.Errorf("cache states: source=%q build=%q", byStage[StageSource].Cache, byStage[StageBuild].Cache)
	}
	for _, v := range views {
		if v.Status != store.TraceStatusDone || v.EndedAt == nil {
			t.Errorf("stage %s not closed: %+v", v.Stage, v)
		}
	}
	tail := StageTail(sink.list(), StageInstall)
	if len(tail) != 2 || tail[0] != "c" || tail[1] != "bare line" {
		t.Errorf("install tail = %v, want last 2 lines", tail)
	}
	if byStage[StageInstall].LogBytes == 0 {
		t.Error("install log bytes not counted")
	}
}

func TestStageTrackerMarksFailedStage(t *testing.T) {
	sink := &stageSink{}
	tracker := NewStageTracker(New(sink, "att2", nil), func() int { return 5 })
	ctx := context.Background()
	tracker.Observe(ctx, StageObs{Step: "[2/3] RUN npm run build", Completed: true, Error: "exit 1"})
	tracker.Finish(ctx)
	views := StagesFromRows(sink.list())
	if len(views) != 1 || views[0].Status != store.TraceStatusFailed {
		t.Fatalf("views = %+v", views)
	}
}

func TestNilStageTrackerIsSafe(_ *testing.T) {
	var tracker *StageTracker
	tracker.Observe(context.Background(), StageObs{Step: "x"})
	tracker.Finish(context.Background())
}
