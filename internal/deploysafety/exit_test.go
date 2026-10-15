package deploysafety

import (
	"testing"
	"time"
)

func TestClassifyExit(t *testing.T) {
	start := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		workload   Workload
		phase      string
		obs        ExitObservation
		wantExited bool
		wantFailed bool
		wantReason string
		wantMsg    string
	}{
		{
			name: "service still running", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs: ExitObservation{Running: true, StartedAt: start},
		},
		{
			name: "service exit 0 before ready is a failure", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 0, StartedAt: start, FinishedAt: start.Add(2 * time.Second)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 0 after 2s, before it became ready",
		},
		{
			name: "service nonzero exit", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 1, StartedAt: start, FinishedAt: start.Add(1400 * time.Millisecond)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 1 after 1s, before it became ready",
		},
		{
			name: "service sub second exit", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 0, StartedAt: start, FinishedAt: start.Add(340 * time.Millisecond)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 0 after 300ms, before it became ready",
		},
		{
			name: "service instant exit", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 0, StartedAt: start, FinishedAt: start.Add(20 * time.Millisecond)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 0 immediately, before it became ready",
		},
		{
			name: "service unknown duration falls back to observed time", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 3, StartedAt: start, ObservedAt: start.Add(5 * time.Second)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 3 after 5s, before it became ready",
		},
		{
			name: "service no timestamps", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 0},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringReadiness,
			wantMsg: "Container exited with code 0, before it became ready",
		},
		{
			name: "service OOM before ready", workload: WorkloadService, phase: ExitPhaseReadiness,
			obs:        ExitObservation{OOMKilled: true, ExitCode: 137, StartedAt: start, FinishedAt: start.Add(4 * time.Second)},
			wantExited: true, wantFailed: true, wantReason: ReasonOOMKilledDuringReadiness,
			wantMsg: "Container was killed for exceeding its memory limit after 4s, before it became ready",
		},
		{
			name: "service exit in stable window", workload: WorkloadService, phase: ExitPhaseWindow,
			obs:        ExitObservation{ExitCode: 0, StartedAt: start, FinishedAt: start.Add(6 * time.Second)},
			wantExited: true, wantFailed: true, wantReason: ReasonExitedDuringWindow,
			wantMsg: "Container exited with code 0 after 6s, during its stable window after becoming ready",
		},
		{
			name: "service OOM in stable window", workload: WorkloadService, phase: ExitPhaseWindow,
			obs:        ExitObservation{OOMKilled: true, StartedAt: start, FinishedAt: start.Add(6 * time.Second)},
			wantExited: true, wantFailed: true, wantReason: ReasonOOMKilledDuringWindow,
			wantMsg: "Container was killed for exceeding its memory limit after 6s, during its stable window after becoming ready",
		},
		{
			name: "job exit 0 is success", workload: WorkloadJob, phase: ExitPhaseReadiness,
			obs:        ExitObservation{ExitCode: 0, StartedAt: start, FinishedAt: start.Add(time.Second)},
			wantExited: true,
		},
		{
			name: "job nonzero exit fails", workload: WorkloadJob,
			obs:        ExitObservation{ExitCode: 2},
			wantExited: true, wantFailed: true, wantMsg: "exited with code 2",
		},
		{
			name: "job OOM fails", workload: WorkloadJob,
			obs:        ExitObservation{OOMKilled: true, ExitCode: 137},
			wantExited: true, wantFailed: true, wantMsg: "container was killed for exceeding its memory limit",
		},
		{
			name: "job still running", workload: WorkloadJob,
			obs: ExitObservation{Running: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyExit(tt.workload, tt.phase, tt.obs)
			if got.Exited != tt.wantExited || got.Failed != tt.wantFailed {
				t.Fatalf("exited=%v failed=%v, want exited=%v failed=%v", got.Exited, got.Failed, tt.wantExited, tt.wantFailed)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}
