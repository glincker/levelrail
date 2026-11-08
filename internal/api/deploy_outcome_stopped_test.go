package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStoppedAppDeployOutcomeIsTerminal(t *testing.T) {
	finished := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	a := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &finished}
	conds := []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: "Suspended", LastTransitionTime: finished.Add(-time.Hour)}}
	if got := computeDeployOutcome(a, true, conds); got != OutcomeStopped {
		t.Fatalf("outcome = %q, want %q", got, OutcomeStopped)
	}
	var res deployAttemptResource
	applyDeployOutcome(&res, a, true, conds)
	if res.OutcomeReason == "" {
		t.Fatal("stopped outcome carries no reason")
	}
	if got := computeDeployOutcome(a, false, conds); got != OutcomeSuperseded {
		t.Fatalf("older attempt outcome = %q, want superseded", got)
	}
}
