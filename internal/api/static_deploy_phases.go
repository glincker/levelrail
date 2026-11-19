package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticPhase is one named step of a static build with its state:
// pending, running, done, failed or unknown (a finished attempt with no stored phase rows).
type staticPhase struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// staticDeployPhases returns the cloning, building, publishing and live steps
// of a static deploy attempt, or nil when the attempt shows no static signal and
// static is false.
func (rt *Router) staticDeployPhases(a store.DeployAttempt, static bool) []staticPhase {
	var live map[string]string
	if a.FinishedAt == nil && rt.deployRecorder != nil {
		live, _ = rt.deployRecorder.StaticPhaseStates(a.ID)
	}
	if live == nil && !static && a.DigestReason != store.DigestReasonStaticTree {
		return nil
	}
	var saved map[string]string
	if a.FinishedAt != nil && a.Status != store.DeployAttemptStatusSucceeded {
		saved = rt.savedStaticPhases(a.ID)
	}
	out := make([]staticPhase, 0, len(deploylog.StaticPhases))
	for _, name := range deploylog.StaticPhases {
		out = append(out, staticPhase{Name: name, State: staticPhaseState(a, live, name)})
	}
	if saved != nil {
		finishedPhaseStates(out, saved, a.Status == store.DeployAttemptStatusFailed)
	}
	return out
}

// savedStaticPhases reads the phase rows a finished attempt left behind.
func (rt *Router) savedStaticPhases(id string) map[string]string {
	out := map[string]string{}
	for _, row := range rt.traceRows(context.Background(), id) {
		if row.Step == deploylog.StepStaticPhase {
			out[row.Detail] = row.Status
		}
	}
	if len(out) == 0 {
		return nil
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

func staticPhaseState(a store.DeployAttempt, live map[string]string, name string) string {
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
