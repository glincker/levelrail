package deploylog

import (
	"context"
	"testing"
)

func TestRecorder_Step_NoopBeforeStart(t *testing.T) {
	r := NewRecorder(nil, nil)
	// No Start call for "missing": Step must not panic and must leave
	// nothing behind for a later SnapshotSteps to find.
	r.Step("missing", "building", "running")

	_, _, _, ok := r.SnapshotSteps("missing")
	if ok {
		t.Fatal("SnapshotSteps(missing) ok = true, want false: Step before Start must be a no-op")
	}
}

func TestRecorder_SnapshotSteps_ReplayThenLive(t *testing.T) {
	r := NewRecorder(nil, nil)
	r.Start("attempt-1")
	r.Step("attempt-1", "detecting", "running")
	r.Step("attempt-1", "detecting", "done")

	steps, live, unsubscribe, ok := r.SnapshotSteps("attempt-1")
	defer unsubscribe()
	if !ok {
		t.Fatal("SnapshotSteps ok = false, want true")
	}
	if len(steps) != 2 || steps[0].Step != "detecting" || steps[0].Status != "running" || steps[1].Status != "done" {
		t.Fatalf("steps = %+v, want [detecting/running detecting/done]", steps)
	}

	r.Step("attempt-1", "building", "running")
	select {
	case ev := <-live:
		if ev.Step != "building" || ev.Status != "running" {
			t.Errorf("live event = %+v, want building/running", ev)
		}
	default:
		t.Fatal("expected a buffered live event after Step, got none")
	}
}

func TestRecorder_Finish_ClosesStepSubscribers(t *testing.T) {
	r := NewRecorder(nil, nil)
	r.Start("attempt-1")
	_, live, unsubscribe, ok := r.SnapshotSteps("attempt-1")
	defer unsubscribe()
	if !ok {
		t.Fatal("SnapshotSteps ok = false, want true")
	}

	r.Finish(context.Background(), "attempt-1")

	if _, chOpen := <-live; chOpen {
		t.Error("live channel still open after Finish, want closed")
	}
}

func TestRecorder_SnapshotSteps_AfterFinish_NotOK(t *testing.T) {
	r := NewRecorder(nil, nil)
	r.Start("attempt-1")
	r.Step("attempt-1", "building", "running")
	r.Finish(context.Background(), "attempt-1")

	_, _, _, ok := r.SnapshotSteps("attempt-1")
	if ok {
		t.Error("SnapshotSteps after Finish ok = true, want false")
	}
}
