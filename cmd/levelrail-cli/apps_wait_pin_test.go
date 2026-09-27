package main

import (
	"context"
	"testing"
	"time"
)

func TestWaitForRollout_PinsLatestAttemptAcrossPolls(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")
	fake := &fakeDeployAttemptFetcher{
		attempts: [][]deployAttemptResource{
			{{ID: "dep_1", Status: "running"}},
			{{ID: "dep_2", Status: "succeeded", FinishedAt: &finished, RolloutState: "serving"}, {ID: "dep_1", Status: "failed", FinishedAt: &finished}},
		},
		conditions: [][]conditionResource{{{Reason: "Deployed", LastTransitionTime: after}}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := waitForRollout(ctx, fake, rolloutWaitConfig{Name: "web", PollInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.state != "failed" {
		t.Errorf("state = %q, want failed for the pinned dep_1, not dep_2's outcome", outcome.state)
	}
}

func TestComputeRolloutOutcome_ServingBeatsLaterFailureCondition(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	later := mustParseRFC3339(t, "2026-01-01T00:05:00Z")
	attempt := deployAttemptResource{ID: "dep_1", Status: "succeeded", FinishedAt: &finished, RolloutState: "serving"}
	got := computeRolloutOutcome(attempt, []conditionResource{{Reason: "ReadinessFailed", LastTransitionTime: later}})
	if got.state != "succeeded" {
		t.Errorf("state = %q, want succeeded: a later unrelated restart must not change this deploy's result", got.state)
	}
}
