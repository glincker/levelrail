package deploysafety

import "github.com/GLINCKER/levelrail/internal/store"

// Stages a release moves through, derived from its gate timeline.
const (
	StagePending      = "pending"
	StageGating       = "gating"
	StageGateFailed   = "gate_failed"
	StageWatching     = "watching"
	StageCleared      = "cleared"
	StageWatchFailed  = "watch_failed"
	StageRolledBack   = "rolled_back"
	StageWatchStopped = "watch_stopped"
)

// Stage folds a timeline (oldest first) into the stage the release is in
// now. A gate failure followed by a passing retry is gating again.
func Stage(events []store.DeployGateEvent) string {
	stage := StagePending
	for _, e := range events {
		switch e.Phase {
		case PhaseReadiness, PhaseStability:
			if e.Status == StatusFailed {
				stage = StageGateFailed
			} else if stage == StagePending || stage == StageGateFailed {
				stage = StageGating
			}
		case PhaseCutover:
			// A crash restart of a live release records another cutover;
			// it must not reopen a watch that already ended.
			if e.Status == StatusPassed && (stage == StagePending || stage == StageGating || stage == StageGateFailed) {
				stage = StageWatching
			}
		case PhaseWatch:
			switch e.Status {
			case StatusPassed:
				stage = StageCleared
			case StatusFailed:
				stage = StageWatchFailed
			case StatusStopped:
				stage = StageWatchStopped
			}
		case PhaseRollback:
			if e.Status == StatusPassed {
				stage = StageRolledBack
			}
		}
	}
	return stage
}

// StageFailed reports whether stage means the release did not hold up.
func StageFailed(stage string) bool {
	switch stage {
	case StageGateFailed, StageWatchFailed, StageRolledBack:
		return true
	}
	return false
}

// StageTerminal reports whether nothing more will happen to a release in
// stage without an operator acting.
func StageTerminal(stage string) bool {
	switch stage {
	case StageCleared, StageGateFailed, StageWatchFailed, StageRolledBack, StageWatchStopped:
		return true
	}
	return false
}
