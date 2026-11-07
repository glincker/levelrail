package deploysafety

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Traffic positions of a release relative to the health gate.
const (
	TrafficHeld     = "held"
	TrafficFlipped  = "flipped"
	TrafficReverted = "reverted"
)

// RolloutState is one release's gate, traffic flip and hold in a single view.
type RolloutState struct {
	Stage   string
	Traffic string
	// GateFailures counts failed readiness or stability checks before the flip.
	GateFailures  int
	Hold          WatchStatus
	AutoRollback  bool
	FlippedAt     time.Time
	RolledBackAt  time.Time
	FailureReason string
}

// RolloutStateOf folds a gate timeline (oldest first) and the effective
// settings into where the release is between the gate and the end of the hold.
func RolloutStateOf(events []store.DeployGateEvent, cfg Effective, now time.Time) RolloutState {
	st := RolloutState{
		Stage: Stage(events), Traffic: TrafficHeld,
		Hold: WatchStatusOf(events, cfg, now), AutoRollback: cfg.AutoRollback,
	}
	for _, e := range events {
		switch {
		case (e.Phase == PhaseReadiness || e.Phase == PhaseStability) && e.Status == StatusFailed:
			st.GateFailures++
			st.FailureReason = e.Reason
		case e.Phase == PhaseCutover && e.Status == StatusPassed && st.FlippedAt.IsZero():
			st.FlippedAt, st.Traffic = e.At, TrafficFlipped
		case e.Phase == PhaseWatch && e.Status == StatusFailed:
			st.FailureReason = e.Reason
		case e.Phase == PhaseRollback && e.Status == StatusPassed:
			st.RolledBackAt, st.Traffic = e.At, TrafficReverted
		}
	}
	return st
}
