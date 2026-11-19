package api

import (
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticPhase is one named step of a static build with its state:
// pending, running, done, failed or unknown (a finished failed attempt keeps no history).
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
	out := make([]staticPhase, 0, len(deploylog.StaticPhases))
	for _, name := range deploylog.StaticPhases {
		out = append(out, staticPhase{Name: name, State: staticPhaseState(a, live, name)})
	}
	return out
}

func staticPhaseState(a store.DeployAttempt, live map[string]string, name string) string {
	switch {
	case a.Status == store.DeployAttemptStatusSucceeded:
		return "done"
	case a.FinishedAt != nil:
		if name == deploylog.PhaseLive {
			return "failed"
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
