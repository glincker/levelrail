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

func TestApplyDeployOutcome_ReasonAndThreshold(t *testing.T) {
	t.Setenv("APP_DEPLOY_ROLLOUT_CRASHLOOP_RESTARTS", "4")
	finished := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	after := finished.Add(time.Minute)
	a := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &finished}

	res := toDeployAttemptResource(a)
	applyDeployOutcome(&res, a, true, []reconcile.Condition{crashLoopCond(5, after)})
	if res.Outcome != OutcomeFailed || res.OutcomeReason == "" {
		t.Errorf("crash loop: outcome %q reason %q, want failed with a reason", res.Outcome, res.OutcomeReason)
	}
	if res.RolloutCrashLoopRestarts != 4 {
		t.Errorf("threshold = %d, want 4 from env", res.RolloutCrashLoopRestarts)
	}

	old := toDeployAttemptResource(a)
	applyDeployOutcome(&old, a, false, nil)
	if old.Outcome != OutcomeSuperseded {
		t.Errorf("older attempt outcome = %q, want superseded", old.Outcome)
	}

	failed := store.DeployAttempt{Status: store.DeployAttemptStatusFailed, Error: "build exited 1"}
	fr := toDeployAttemptResource(failed)
	applyDeployOutcome(&fr, failed, true, nil)
	if fr.Outcome != OutcomeFailed || fr.OutcomeReason != "build exited 1" {
		t.Errorf("failed: outcome %q reason %q", fr.Outcome, fr.OutcomeReason)
	}
}
