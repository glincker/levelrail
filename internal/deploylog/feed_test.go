package deploylog

import (
	"context"
	"testing"
)

func TestSubscribeAllAndStepSummary(t *testing.T) {
	r := NewRecorder(nil, nil)
	events, unsubscribe := r.SubscribeAll()
	defer unsubscribe()

	r.Start("dep_1")
	r.Step("dep_1", "cloning", "running")
	r.Step("dep_1", "cloning", "done")
	r.Step("dep_1", "building", "running")
	r.Step("dep_1", "building", "failed")

	sum, ok := r.StepSummaryFor("dep_1")
	if !ok {
		t.Fatal("expected a live attempt")
	}
	if sum.Done != 1 || sum.Failed != 1 || sum.Running != 0 || sum.FailingStep != "building" {
		t.Errorf("summary = %+v", sum)
	}

	r.Finish(context.Background(), "dep_1")
	if _, ok := r.StepSummaryFor("dep_1"); ok {
		t.Error("finished attempt should have no live step summary")
	}

	want := []string{StateStarted, StateStep, StateStep, StateStep, StateStep, StateFinished}
	for i, kind := range want {
		select {
		case ev := <-events:
			if ev.Kind != kind || ev.AttemptID != "dep_1" {
				t.Errorf("event %d = %+v, want kind %s", i, ev, kind)
			}
		default:
			t.Fatalf("missing event %d (%s)", i, kind)
		}
	}
}
