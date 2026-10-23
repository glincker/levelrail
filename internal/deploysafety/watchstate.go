package deploysafety

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Watch states a release's post-deploy watch can be in.
const (
	WatchStateOff        = "off"
	WatchStateWaiting    = "waiting"
	WatchStateWatching   = "watching"
	WatchStateCleared    = "cleared"
	WatchStateTripped    = "tripped"
	WatchStateRolledBack = "rolled_back"
	WatchStateStopped    = "stopped"
)

// WatchStatus is the operator-facing summary of one release's watch.
type WatchStatus struct {
	State         string
	Mode          string
	StartedAt     time.Time
	EndsAt        time.Time
	RemainingSecs int
	WindowSecs    int
	// Reason and Message describe the trip or the rollback; Notice carries a
	// soft signal that only alerted.
	Reason  string
	Message string
	Notice  string
}

// WatchStatusOf folds a release's gate timeline (oldest first) into its
// watch status at now.
func WatchStatusOf(events []store.DeployGateEvent, cfg Effective, now time.Time) WatchStatus {
	st := WatchStatus{State: WatchStateWaiting, Mode: cfg.WatchMode, WindowSecs: int(cfg.WatchWindow / time.Second)}
	for _, e := range events {
		switch {
		case e.Phase == PhaseCutover && e.Status == StatusPassed && st.StartedAt.IsZero():
			st.StartedAt = e.At
			st.EndsAt = e.At.Add(cfg.WatchWindow)
		case e.Phase == PhaseWatch && e.Status == StatusFailed:
			st.State, st.Reason, st.Message = WatchStateTripped, e.Reason, e.Message
		case e.Phase == PhaseWatch && e.Status == StatusPassed:
			st.State = WatchStateCleared
		case e.Phase == PhaseWatch && e.Status == StatusStopped:
			st.State, st.Message = WatchStateStopped, e.Message
		case e.Phase == PhaseWatch && e.Status == StatusInfo:
			st.Notice = e.Message
		case e.Phase == PhaseRollback && e.Status == StatusPassed:
			st.State = WatchStateRolledBack
		}
	}
	terminal := st.State != WatchStateWaiting
	switch {
	case !terminal && cfg.WatchMode == WatchModeOff:
		st.State = WatchStateOff
	case !terminal && !st.StartedAt.IsZero():
		st.State = WatchStateWatching
		if left := st.EndsAt.Sub(now); left > 0 {
			st.RemainingSecs = int(left / time.Second)
		}
	}
	return st
}
