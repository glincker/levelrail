package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComputeDeployOutcome(t *testing.T) {
	finished := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	before, after := finished.Add(-time.Minute), finished.Add(time.Minute)
	ok := func(mod func(*store.DeployAttempt)) store.DeployAttempt {
		a := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &finished}
		if mod != nil {
			mod(&a)
		}
		return a
	}
	tests := []struct {
		name   string
		a      store.DeployAttempt
		newest bool
		conds  []reconcile.Condition
		want   string
	}{
		{"failed", store.DeployAttempt{Status: store.DeployAttemptStatusFailed}, true, nil, OutcomeFailed},
		{"canceled", store.DeployAttempt{Status: store.DeployAttemptStatusCanceled}, true, nil, OutcomeCanceled},
		{"superseded", store.DeployAttempt{Status: store.DeployAttemptStatusSuperseded}, false, nil, OutcomeSuperseded},
		{"held", store.DeployAttempt{Status: store.DeployAttemptStatusHeld}, true, nil, OutcomeBlocked},
		{"running", store.DeployAttempt{Status: store.DeployAttemptStatusRunning}, true, nil, OutcomeInProgress},
		{"queued", store.DeployAttempt{Status: store.DeployAttemptStatusQueued}, true, nil, OutcomeInProgress},
		{"succeeded, no rollout signal yet", ok(nil), true, []reconcile.Condition{{Reason: "Deployed", LastTransitionTime: before}}, OutcomeInProgress},
		{"succeeded, done condition after finish", ok(nil), true, []reconcile.Condition{{Reason: "Deployed", LastTransitionTime: after}}, OutcomeHealthy},
		{"succeeded, readiness failed", ok(nil), true, []reconcile.Condition{{Reason: "ReadinessFailed", LastTransitionTime: after}}, OutcomeFailed},
		{"serving is healthy even with a later failure condition", ok(func(a *store.DeployAttempt) { a.RolloutState = store.RolloutStateServing }), true, []reconcile.Condition{{Reason: "ReadinessFailed", LastTransitionTime: after}}, OutcomeHealthy},
		{"older succeeded attempt never serving", ok(nil), false, []reconcile.Condition{{Reason: "ReadinessFailed", LastTransitionTime: after}}, OutcomeSuperseded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeDeployOutcome(tc.a, tc.newest, tc.conds); got != tc.want {
				t.Errorf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}
