package deploytrace

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type memSink struct {
	mu    sync.Mutex
	steps map[string]store.DeployTraceStep
}

func (m *memSink) SaveDeployTraceStep(_ context.Context, s store.DeployTraceStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.steps == nil {
		m.steps = map[string]store.DeployTraceStep{}
	}
	m.steps[s.AttemptID+"/"+s.Key] = s
	return nil
}

func TestNilTracerIsInert(t *testing.T) {
	var tr *Tracer
	span := tr.Begin(context.Background(), "x", "d")
	span.Progress(context.Background(), "p")
	span.Done(context.Background(), "d")
	span.Fail(context.Background(), "R", errors.New("e"))
	tr.Note(context.Background(), "x", store.TraceStatusDone, "", "")
	if tr.WithService("a") != nil || tr.AttemptID() != "" || tr.Clone() != nil || tr.Has("x") {
		t.Error("nil tracer helpers should return zero values")
	}
	if New(nil, "dep", nil) != nil || New(&memSink{}, "", nil) != nil {
		t.Error("New without a sink or attempt should return nil")
	}
}

func TestSpanLifecycle(t *testing.T) {
	sink := &memSink{}
	tr := New(sink, "dep_1", nil).WithService("web")
	now := time.Unix(1000, 0)
	tr.now = func() time.Time { return now }

	span := tr.Begin(context.Background(), StepImagePull, "pulling")
	if s := sink.steps["dep_1/image_pull"]; s.Status != store.TraceStatusRunning || s.Service != "web" {
		t.Fatalf("running step = %+v", s)
	}
	now = now.Add(2500 * time.Millisecond)
	span.Fail(context.Background(), "ImagePullFailed", errors.New("unexpected EOF"))
	s := sink.steps["dep_1/image_pull"]
	if s.Status != store.TraceStatusFailed || s.Reason != "ImagePullFailed" || s.Detail != "unexpected EOF" || s.DurationMS != 2500 {
		t.Errorf("failed step = %+v, want failed with trimmed Docker text and 2500ms", s)
	}
}

func TestBufferedFlushTargetsEachAttempt(t *testing.T) {
	tr := NewBuffered()
	tr.Note(context.Background(), StepComposePlan, store.TraceStatusDone, "", "plan")
	span := tr.Begin(context.Background(), StepComposeParse, "parsing")
	span.Done(context.Background(), "ok")
	per := tr.Clone()
	per.Note(context.Background(), StepComposeService, store.TraceStatusDone, "", "web")

	sink := &memSink{}
	tr.Flush(context.Background(), sink, "dep_a", "web")
	per.Flush(context.Background(), sink, "dep_b", "db")
	if len(sink.steps) != 5 {
		t.Fatalf("steps = %d (%v), want 2 shared steps for a plus 3 for b", len(sink.steps), sink.steps)
	}
	if s := sink.steps["dep_a/compose_parse"]; s.Status != store.TraceStatusDone || s.Service != "web" {
		t.Errorf("flushed parse = %+v, want the span's final state labelled web", s)
	}
	if _, leaked := sink.steps["dep_a/compose_service"]; leaked {
		t.Error("a clone's step leaked into the original tracer")
	}
	if !per.Has(StepComposeService) || tr.Has(StepComposeService) {
		t.Error("Has() disagrees with the clone split")
	}
}

func TestFormatLine(t *testing.T) {
	tests := []struct {
		name string
		in   store.DeployTraceStep
		want []string
	}{
		{"failed with reason and duration", store.DeployTraceStep{Step: StepImagePull, Key: StepImagePull, Status: "failed", Reason: "ImagePullFailed", Detail: "unexpected EOF", DurationMS: 1200},
			[]string{"[trace] image_pull: failed", "[ImagePullFailed]", "unexpected EOF", "(1.2s)"}},
		{"keyed probe with service", store.DeployTraceStep{Step: StepReadinessProbe, Key: "readiness_probe.3", Service: "web", Status: "warning"},
			[]string{"readiness_probe (web) 3: warning"}},
		{"running has no duration", store.DeployTraceStep{Step: StepCutover, Key: StepCutover, Status: "running", DurationMS: 50},
			[]string{"cutover: running"}},
	}
	for _, tt := range tests {
		got := FormatLine(tt.in)
		for _, w := range tt.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q missing %q", tt.name, got, w)
			}
		}
	}
	if strings.Contains(FormatLine(store.DeployTraceStep{Step: "x", Status: "running", DurationMS: 50}), "(50ms)") {
		t.Error("a running step should not print a duration")
	}
}

func TestEnvSummaryNeverPrintsValues(t *testing.T) {
	got := EnvSummary([]string{"PORT", "TOKEN"}, map[string]string{"TOKEN": "secret", "DB": "database"})
	for _, want := range []string{"3 env var(s)", "TOKEN (secret, masked)", "DB (database, masked)", "PORT"} {
		if !strings.Contains(got, want) {
			t.Errorf("EnvSummary = %q, missing %q", got, want)
		}
	}
	if EnvSummary(nil, nil) != "no env vars" {
		t.Error("empty env should say so")
	}
}

func TestBytes(t *testing.T) {
	for in, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 5 << 20: "5.0 MiB", 3 << 30: "3.0 GiB"} {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", in, got, want)
		}
	}
}
