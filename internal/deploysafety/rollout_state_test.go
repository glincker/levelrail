package deploysafety

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRolloutStateOf(t *testing.T) {
	cfg := BuiltinDefaults()
	cfg.WatchMode, cfg.AutoRollback, cfg.WatchWindow = WatchModeAuto, true, 5*time.Minute
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(phase, status, reason string, d time.Duration) store.DeployGateEvent {
		return store.DeployGateEvent{Phase: phase, Status: status, Reason: reason, At: t0.Add(d)}
	}
	tests := []struct {
		name    string
		events  []store.DeployGateEvent
		traffic string
		stage   string
		fails   int
		remain  int
	}{
		{"gate failing keeps traffic held", []store.DeployGateEvent{at(PhaseReadiness, StatusFailed, "unready", 0)}, TrafficHeld, StageGateFailed, 1, 0},
		{"flipped and holding", []store.DeployGateEvent{at(PhaseReadiness, StatusPassed, "", 0), at(PhaseCutover, StatusPassed, "", 0)}, TrafficFlipped, StageWatching, 0, 240},
		{"hold tripped then reverted", []store.DeployGateEvent{at(PhaseCutover, StatusPassed, "", 0), at(PhaseWatch, StatusFailed, "errors", time.Minute), at(PhaseRollback, StatusPassed, "", 2*time.Minute)}, TrafficReverted, StageRolledBack, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RolloutStateOf(tt.events, cfg, t0.Add(time.Minute))
			if got.Traffic != tt.traffic || got.Stage != tt.stage || got.GateFailures != tt.fails || got.Hold.RemainingSecs != tt.remain {
				t.Fatalf("got %+v, want traffic %s stage %s fails %d remain %d", got, tt.traffic, tt.stage, tt.fails, tt.remain)
			}
		})
	}
}
