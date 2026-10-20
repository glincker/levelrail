package deploysafety

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func ev(phase, status string) store.DeployGateEvent {
	return store.DeployGateEvent{Phase: phase, Status: status}
}

func TestStage(t *testing.T) {
	tests := []struct {
		name   string
		events []store.DeployGateEvent
		want   string
	}{
		{"nothing yet", nil, StagePending},
		{"readiness passed", []store.DeployGateEvent{ev(PhaseReadiness, StatusPassed)}, StageGating},
		{"stability failed", []store.DeployGateEvent{ev(PhaseReadiness, StatusPassed), ev(PhaseStability, StatusFailed)}, StageGateFailed},
		{"retry after failure", []store.DeployGateEvent{ev(PhaseStability, StatusFailed), ev(PhaseReadiness, StatusPassed)}, StageGating},
		{"cut over", []store.DeployGateEvent{ev(PhaseStability, StatusPassed), ev(PhaseCutover, StatusPassed)}, StageWatching},
		{"watch cleared", []store.DeployGateEvent{ev(PhaseCutover, StatusPassed), ev(PhaseWatch, StatusPassed)}, StageCleared},
		{"watch tripped, no rollback", []store.DeployGateEvent{ev(PhaseCutover, StatusPassed), ev(PhaseWatch, StatusFailed), ev(PhaseRollback, StatusFailed)}, StageWatchFailed},
		{"cleared, later restart cutover ignored", []store.DeployGateEvent{ev(PhaseCutover, StatusPassed), ev(PhaseWatch, StatusPassed), ev(PhaseCutover, StatusPassed)}, StageCleared},
		{"rolled back, later restart cutover ignored", []store.DeployGateEvent{ev(PhaseWatch, StatusFailed), ev(PhaseRollback, StatusPassed), ev(PhaseCutover, StatusPassed)}, StageRolledBack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Stage(tt.events); got != tt.want {
				t.Errorf("Stage() = %q, want %q", got, tt.want)
			}
		})
	}
	if StageTerminal(StageWatching) || !StageTerminal(StageRolledBack) {
		t.Error("StageTerminal() misclassifies watching or rolled_back")
	}
	if StageFailed(StageCleared) || !StageFailed(StageWatchFailed) || !StageFailed(StageGateFailed) {
		t.Error("StageFailed() misclassifies cleared, watch_failed or gate_failed")
	}
}
