package nodeinsight

// Agent update actions, stable for the agent and the dashboard.
const (
	UpdateNone     = "none"
	UpdateInstall  = "update"
	UpdateRollback = "rollback"
	UpdateHold     = "hold"
)

// UpdateFacts is what the control plane knows when an agent asks for its target.
type UpdateFacts struct {
	Channel        string
	ControlPlane   string
	AgentVersion   string
	AgentOnline    bool
	PreviousGood   string
	HealthFailures int
	// MaxHealthFailures is the runtime setting after which an update is undone.
	MaxHealthFailures int
}

// UpdateDecision is the answer to an agent update check. The agent only ever
// installs a signed release it fetches itself; this never carries a command.
type UpdateDecision struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Target  string `json:"target_version,omitempty"`
	Reason  string `json:"reason"`
}

// DecideUpdate keeps agents on the control plane version of the control plane
// channel and rolls back to the last good version when health keeps failing.
func DecideUpdate(f UpdateFacts) UpdateDecision {
	d := UpdateDecision{Action: UpdateNone, Channel: f.Channel}
	if f.MaxHealthFailures > 0 && f.HealthFailures >= f.MaxHealthFailures && f.PreviousGood != "" {
		d.Action, d.Target, d.Reason = UpdateRollback, f.PreviousGood, "health checks failed after the last update"
		return d
	}
	switch Classify(f.AgentVersion, f.ControlPlane) {
	case SkewBehind:
		if !f.AgentOnline {
			d.Action, d.Reason = UpdateHold, "agent is offline"
			return d
		}
		d.Action, d.Target, d.Reason = UpdateInstall, f.ControlPlane, "agent is behind the control plane"
	case SkewUnknown:
		d.Reason = "agent version is unknown"
	default:
		d.Reason = "agent matches the control plane"
	}
	return d
}
