package deploylog

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/store"
)

type phaseSink struct {
	mu   sync.Mutex
	rows []store.DeployTraceStep
}

func (s *phaseSink) SaveDeployTraceStep(_ context.Context, row store.DeployTraceStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, row)
	return nil
}

func TestBuildingNotDoneBeforeCollect(t *testing.T) {
	r := NewRecorder(nil, nil)
	r.Start("a1")
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Build static site", Completed: true})
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Collect build output"})
	got, _ := r.StaticPhaseStates("a1")
	if got[PhaseBuilding] != "running" {
		t.Fatalf("building = %q, want running", got[PhaseBuilding])
	}
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Collect build output", Completed: true})
	got, _ = r.StaticPhaseStates("a1")
	if got[PhaseBuilding] != "done" {
		t.Fatalf("building = %q, want done", got[PhaseBuilding])
	}
}

func TestStaticPhasesStayOutOfStepViews(t *testing.T) {
	r := NewRecorder(nil, nil)
	events, unsub := r.SubscribeAll()
	defer unsub()
	r.Start("a1")
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Fetch source"})
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Build static site", Error: "boom"})
	sum, _ := r.StepSummaryFor("a1")
	if sum.Running != 0 || sum.Failed != 0 || sum.FailingStep != "" {
		t.Fatalf("summary = %+v, want no phase steps", sum)
	}
	if got, _ := r.StaticPhaseStates("a1"); got[PhaseBuilding] != "failed" {
		t.Fatalf("phase states = %v", got)
	}
	for len(events) > 0 {
		if ev := <-events; ev.Kind == StateStep {
			t.Fatalf("state feed saw step %q", ev.Step)
		}
	}
}

func TestStaticPhasePersistsOnChangeWithStart(t *testing.T) {
	sink := &phaseSink{}
	r := NewRecorder(nil, nil)
	r.SetStages(sink, func() int { return 0 })
	r.Start("a1")
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Fetch source"})
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Fetch source", Log: "line"})
	time.Sleep(2 * time.Millisecond)
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Fetch source", Completed: true})
	r.noteStaticPhase("a1", build.ProgressEvent{Step: "Collect build output", Completed: true})
	if len(sink.rows) != 3 {
		t.Fatalf("writes = %d, want 3 (running, done, building done)", len(sink.rows))
	}
	clone, built := sink.rows[1], sink.rows[2]
	if clone.StartedAt.IsZero() || !clone.EndedAt.After(clone.StartedAt) {
		t.Fatalf("cloning times = %v..%v", clone.StartedAt, clone.EndedAt)
	}
	if !built.StartedAt.Equal(clone.EndedAt) {
		t.Fatalf("building started %v, want previous end %v", built.StartedAt, clone.EndedAt)
	}
}
