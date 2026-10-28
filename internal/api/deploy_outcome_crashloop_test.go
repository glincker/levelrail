package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func crashLoopCond(restarts int, at time.Time) reconcile.Condition {
	return reconcile.Condition{
		Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: deploysafety.ReasonCrashLoopBackOff,
		Message: deploysafety.CrashLoopMessage("web-abc", restarts, 4*time.Second), LastTransitionTime: at,
	}
}

func TestComputeDeployOutcome_CrashLoop(t *testing.T) {
	t.Setenv("APP_DEPLOY_ROLLOUT_CRASHLOOP_RESTARTS", "")
	finished := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	after := finished.Add(time.Minute)
	pending := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &finished}
	serving := pending
	serving.RolloutState = store.RolloutStateServing
	tests := []struct {
		name  string
		a     store.DeployAttempt
		conds []reconcile.Condition
		want  string
	}{
		{"first deploy keeps crash looping", pending, []reconcile.Condition{crashLoopCond(3, after)}, OutcomeFailed},
		{"one restart is not yet a crash loop", pending, []reconcile.Condition{crashLoopCond(1, after)}, OutcomeInProgress},
		{"crash then recovery", pending, []reconcile.Condition{{Reason: "Deployed", LastTransitionTime: after.Add(time.Minute)}}, OutcomeHealthy},
		{"steady-state crash after success", serving, []reconcile.Condition{crashLoopCond(5, after)}, OutcomeHealthy},
		{"crash loop before this attempt finished", pending, []reconcile.Condition{crashLoopCond(5, finished.Add(-time.Minute))}, OutcomeInProgress},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeDeployOutcome(tc.a, true, tc.conds); got != tc.want {
				t.Errorf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
	if got := rolloutFailureCondition(pending, []reconcile.Condition{crashLoopCond(3, after)}); got == nil || got.Reason != deploysafety.ReasonCrashLoopBackOff {
		t.Errorf("failure condition = %+v, want the CrashLoopBackOff reason shown", got)
	}
	if got := rolloutFailureCondition(serving, []reconcile.Condition{crashLoopCond(5, after)}); got != nil {
		t.Errorf("steady-state crash attached failure %+v to a serving attempt", got)
	}
}
