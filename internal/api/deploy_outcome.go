package api

import (
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Deploy outcomes reported by GET /apps/{name}/deploys/{deployId}.
const (
	OutcomeInProgress = "in_progress"
	OutcomeHealthy    = "healthy"
	OutcomeFailed     = "failed"
	OutcomeCanceled   = "canceled"
	OutcomeSuperseded = "superseded"
	OutcomeBlocked    = "blocked"
)

var rolloutDoneReasons = map[string]bool{"Deployed": true, "AlreadyRunning": true}

// computeDeployOutcome derives one deploy's outcome. It is keyed on the
// attempt: a serving rollout state is proof this attempt's image is running,
// and rollout conditions only count for the app's newest attempt, so a later
// restart or deploy cannot be mistaken for this deploy's result.
func computeDeployOutcome(a store.DeployAttempt, newest bool, conds []reconcile.Condition) string {
	switch a.Status {
	case store.DeployAttemptStatusFailed:
		return OutcomeFailed
	case store.DeployAttemptStatusCanceled:
		return OutcomeCanceled
	case store.DeployAttemptStatusSuperseded:
		return OutcomeSuperseded
	case store.DeployAttemptStatusHeld:
		return OutcomeBlocked
	case store.DeployAttemptStatusSucceeded:
	default:
		return OutcomeInProgress
	}
	if a.RolloutState == store.RolloutStateServing {
		return OutcomeHealthy
	}
	if !newest {
		return OutcomeSuperseded
	}
	if a.FinishedAt == nil {
		return OutcomeInProgress
	}
	if rolloutFailureCondition(a, conds) != nil {
		return OutcomeFailed
	}
	for _, c := range conds {
		if rolloutDoneReasons[c.Reason] && !c.LastTransitionTime.Before(*a.FinishedAt) {
			return OutcomeHealthy
		}
	}
	return OutcomeInProgress
}
