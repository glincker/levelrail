package api

import (
	"context"

	"github.com/glincker/levelrail/internal/deploylog"
	"github.com/glincker/levelrail/internal/store"
)

// staticPhase is one named step of a static build with its state:
// pending, running, done, failed or unknown (a finished attempt with no stored phase rows).
type staticPhase struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// phaseRowLister loads the static phase rows of many attempts in one query.
type phaseRowLister interface {
	ListDeployTraceStepsByStep(ctx context.Context, step string, attemptIDs []string) (map[string][]store.DeployTraceStep, error)
}

// staticDeployPhases returns the cloning, building, publishing and live steps
// of a static deploy attempt, or nil when the attempt shows no static signal:
// stored phase rows, the static-tree digest marker or static being true.
func (rt *Router) staticDeployPhases(a store.DeployAttempt, static bool) []staticPhase {
	return rt.staticPhasesFrom(a, static, nil)
}

// staticPhasesFrom is staticDeployPhases with optional preloaded phase rows
// (keyed by attempt id) so a list does not query once per attempt.
func (rt *Router) staticPhasesFrom(a store.DeployAttempt, static bool, preloaded map[string][]store.DeployTraceStep) []staticPhase {
	var live map[string]string
	if a.FinishedAt == nil && rt.deployRecorder != nil {
		live, _ = rt.deployRecorder.StaticPhaseStates(a.ID)
	}
	var saved map[string]string
	if a.FinishedAt != nil {
		rows, ok := preloaded[a.ID]
		if !ok && preloaded == nil {
			rows = rt.staticPhaseRows(a.ID)
		}
		saved = phaseStatuses(rows)
	}
	if live == nil && saved == nil && !static && a.DigestReason != store.DigestReasonStaticTree {
		return nil
	}
	out := make([]staticPhase, 0, len(deploylog.StaticPhases))
	for _, name := range deploylog.StaticPhases {
		out = append(out, staticPhaseState(a, live, name))
	}
	if saved != nil && a.Status != store.DeployAttemptStatusSucceeded {
		finishedPhaseStates(out, saved, a.Status == store.DeployAttemptStatusFailed)
	}
	return out
}

// staticPhaseRows reads the phase rows one finished attempt left behind.
func (rt *Router) staticPhaseRows(id string) []store.DeployTraceStep {
	var out []store.DeployTraceStep
	for _, row := range rt.allTraceRows(context.Background(), id) {
		if row.Step == deploylog.StepStaticPhase {
			out = append(out, row)
		}
	}
	return out
}

// preloadStaticPhaseRows fetches phase rows for every finished attempt in
// one query; nil means the store cannot batch and callers load per attempt.
func (rt *Router) preloadStaticPhaseRows(ctx context.Context, rows []store.Deployment) map[string][]store.DeployTraceStep {
	l, ok := rt.deployAttempts.(phaseRowLister)
	if !ok {
		return nil
	}
	var ids []string
	for _, d := range rows {
		if d.Attempt.FinishedAt != nil {
			ids = append(ids, d.Attempt.ID)
		}
	}
	out, err := l.ListDeployTraceStepsByStep(ctx, deploylog.StepStaticPhase, ids)
	if err != nil {
		rt.logger.Warn("api: list static phase rows failed", "error", err.Error())
		return nil
	}
	return out
}

func phaseStatuses(rows []store.DeployTraceStep) map[string]string {
	if len(rows) == 0 {
		return nil
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Detail] = row.Status
	}
	return out
}

// finishedPhaseStates maps stored phase rows onto a finished attempt: phases
// up to the first one that is not done keep their state, that one is the
// failing step (pending when the attempt did not fail), the rest are pending.
func finishedPhaseStates(phases []staticPhase, saved map[string]string, failed bool) {
	broken := false
	for i := range phases {
		switch {
		case broken || phases[i].Name == deploylog.PhaseLive:
			phases[i].State = "pending"
		case saved[phases[i].Name] == "done":
			phases[i].State = "done"
		default:
			broken = true
			phases[i].State = "pending"
			if failed {
				phases[i].State = "failed"
			}
		}
	}
}

func staticPhaseState(a store.DeployAttempt, live map[string]string, name string) staticPhase {
	return staticPhase{Name: name, State: staticPhaseStateName(a, live, name)}
}

func staticPhaseStateName(a store.DeployAttempt, live map[string]string, name string) string {
	switch {
	case a.Status == store.DeployAttemptStatusSucceeded:
		return "done"
	case a.FinishedAt != nil:
		if name == deploylog.PhaseLive {
			return "pending"
		}
		return "unknown"
	case name == deploylog.PhaseLive:
		return "pending"
	}
	if s, ok := live[name]; ok {
		return s
	}
	return "pending"
}
