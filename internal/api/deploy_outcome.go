package api

import (
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
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

// Non-terminal and derived statuses reported by the single-deploy endpoint.
const (
	StatusBuilding   = "building"
	StatusRollingOut = "rolling_out"
)

// presentedDeployStatus maps the stored attempt status to what a poller
// should see: "succeeded" is stored once the request is accepted, but it is
// only reported once the rollout gate passed.
func presentedDeployStatus(stored, outcome string) string {
	switch {
	case stored == store.DeployAttemptStatusRunning:
		return StatusBuilding
	case stored != store.DeployAttemptStatusSucceeded:
		return stored
	case outcome == OutcomeInProgress:
		return StatusRollingOut
	case outcome == OutcomeFailed:
		return store.DeployAttemptStatusFailed
	}
	return stored
}

// Completed is only emitted for a run-to-completion service that exited 0.
var rolloutDoneReasons = map[string]bool{"Deployed": true, "AlreadyRunning": true, "StaticSitePublished": true, application.ReasonCompleted: true}

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
	case store.DeployAttemptStatusHeld, store.DeployAttemptStatusAwaitingApproval:
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

// applyDeployOutcome sets the outcome, its short reason and the crash-loop
// threshold on res, so every response that carries an attempt agrees.
func applyDeployOutcome(res *deployAttemptResource, a store.DeployAttempt, newest bool, conds []reconcile.Condition) {
	res.Outcome = computeDeployOutcome(a, newest, conds)
	res.OutcomeReason = deployOutcomeReason(a, res.Outcome, conds)
	res.RolloutCrashLoopRestarts = failure.RolloutCrashLoopRestarts(os.LookupEnv)
}

func deployOutcomeReason(a store.DeployAttempt, outcome string, conds []reconcile.Condition) string {
	switch outcome {
	case OutcomeFailed:
		if c := rolloutFailureCondition(a, conds); c != nil {
			return outcomeReasonLine(c.Message, c.Reason)
		}
		return outcomeReasonLine(a.Error, "")
	case OutcomeHealthy:
		return "serving"
	case OutcomeInProgress:
		if a.Status == store.DeployAttemptStatusSucceeded {
			return "rolling out"
		}
		return "building"
	case OutcomeSuperseded:
		return "replaced by a newer deploy"
	case OutcomeCanceled:
		return "canceled"
	}
	return ""
}

func outcomeReasonLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return fallback
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
