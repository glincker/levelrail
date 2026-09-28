package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const latestDeployID = "latest"

func failureOptions() failure.Options { return failure.OptionsFromEnv(os.LookupEnv) }

func (rt *Router) failingStep(id string) string {
	if rt.deployRecorder == nil {
		return ""
	}
	if sum, ok := rt.deployRecorder.StepSummaryFor(id); ok {
		return sum.FailingStep
	}
	return ""
}

func attemptFailureInput(a store.DeployAttempt, failingStep string) failure.Input {
	at := a.StartedAt
	if a.FinishedAt != nil {
		at = *a.FinishedAt
	}
	return failure.Input{
		App: a.ServiceName, DeployID: a.ID, Status: a.Status, Reason: a.Reason,
		Error: a.Error, FailingStep: failingStep, At: at,
	}
}

// attachFailure sets res.Failure from the attempt row alone, with no log or
// condition lookups, so list endpoints stay cheap.
func (rt *Router) attachFailure(res *deployAttemptResource, a store.DeployAttempt) {
	if a.Status != store.DeployAttemptStatusFailed && a.Status != store.DeployAttemptStatusHeld {
		return
	}
	if f, ok := failure.Classify(attemptFailureInput(a, rt.failingStep(a.ID)), failureOptions()); ok {
		res.Failure = &f
	}
}

// rolloutFailureCondition returns the failing condition a succeeded attempt's
// rollout hit after the attempt finished, if any.
func rolloutFailureCondition(a store.DeployAttempt, conds []reconcile.Condition) *failure.Condition {
	if a.Status != store.DeployAttemptStatusSucceeded || a.FinishedAt == nil {
		return nil
	}
	for _, c := range conds {
		if failure.RolloutFailureReasons[c.Reason] && !c.LastTransitionTime.Before(*a.FinishedAt) {
			return &failure.Condition{Reason: c.Reason, Message: c.Message}
		}
	}
	return nil
}

// classifyAttempt classifies attempt with the full signal set: build or
// runtime logs, the crashloop state and, for the app's newest attempt, the
// current rollout conditions.
func (rt *Router) classifyAttempt(ctx context.Context, a store.DeployAttempt, conds []reconcile.Condition, newest bool) (*failure.Failure, bool) {
	in := attemptFailureInput(a, rt.failingStep(a.ID))
	if newest {
		in.Condition = rolloutFailureCondition(a, conds)
	}
	if in.Status != store.DeployAttemptStatusFailed && in.Status != store.DeployAttemptStatusHeld && in.Condition == nil {
		return nil, false
	}
	if attemptFailedDuringBuild(&a) {
		in.LogLines = rt.diagnoseBuildLogs(ctx, a.ServiceName, a.ID)
	} else if in.Status == store.DeployAttemptStatusFailed || in.Condition != nil {
		in.LogLines = rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	if newest {
		if c := rt.diagnoseCrashloop(ctx, a.ServiceName); c != nil {
			in.Crashloop = c.Firing
		}
	}
	f, ok := failure.Classify(in, failureOptions())
	if !ok {
		return nil, false
	}
	return &f, true
}

// handleGetDeploy handles GET /api/v1/apps/{name}/deploys/{deployId}: one
// deploy attempt with its structured failure, if it did not succeed.
// deployId may be "latest".
func (rt *Router) handleGetDeploy(w http.ResponseWriter, r *http.Request) {
	name, id := r.PathValue("name"), r.PathValue("deployId")
	ctx := r.Context()

	if _, err := rt.apps.GetDesiredService(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get deploy: load app", err)
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.internalError(w, "api: get deploy: list attempts", err)
		return
	}
	idx := -1
	for i, a := range attempts {
		if a.ID == id || (id == latestDeployID && i == 0) {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	a := attempts[idx]
	conds, err := rt.deploys.GetConditions(ctx, applicationControllerName(name))
	if err != nil {
		rt.logger.Warn("api: get deploy: load conditions failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("deploy_id", a.ID))
	}

	res := toDeployAttemptResource(a)
	rt.applyWait(ctx, &res, a, attempts)
	res.Outcome = computeDeployOutcome(a, idx == 0, conds)
	if f, ok := rt.classifyAttempt(ctx, a, conds, idx == 0); ok {
		res.Failure = f
	}
	writeJSON(w, http.StatusOK, res)
}

// diagnoseFailure classifies the attempt diagnose already loaded, reusing the
// logs and crashloop state it collected. With no failed attempt, a firing
// crashloop or failing rollout condition still yields a failure.
func (rt *Router) diagnoseFailure(attempt *store.DeployAttempt, conds []reconcile.Condition, din diagnose.Input) *failure.Failure {
	in := failure.Input{Crashloop: din.Crashloop != nil && din.Crashloop.Firing, LogLines: din.RecentLogLines, At: time.Now()}
	if attempt != nil {
		in = attemptFailureInput(*attempt, rt.failingStep(attempt.ID))
		in.Crashloop = din.Crashloop != nil && din.Crashloop.Firing
		in.LogLines = din.RecentLogLines
		in.Condition = rolloutFailureCondition(*attempt, conds)
	} else {
		in.App = din.ServiceName
	}
	if in.Status == "" && in.Condition == nil && in.Crashloop {
		in.Status = store.DeployAttemptStatusFailed
	}
	f, ok := failure.Classify(in, failureOptions())
	if !ok {
		return nil
	}
	return &f
}
