package deploysafety

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEvaluateGate(t *testing.T) {
	cfg := Effective{SuccessThreshold: 3, StableWindow: 10 * time.Second}
	start := time.Unix(1000, 0)
	tests := []struct {
		name       string
		obs        GateObservation
		want       Verdict
		wantReason string
	}{
		{"running, enough passes, window elapsed", GateObservation{Running: true, ConsecutivePasses: 3, SinceStart: 10 * time.Second}, VerdictPass, ""},
		{"enough passes but window not elapsed", GateObservation{Running: true, ConsecutivePasses: 5, SinceStart: 9 * time.Second}, VerdictPending, ""},
		{"window elapsed but too few passes", GateObservation{Running: true, ConsecutivePasses: 2, SinceStart: time.Minute}, VerdictPending, ""},
		{"oom killed beats everything", GateObservation{Running: true, OOMKilled: true, ConsecutivePasses: 9, SinceStart: time.Minute}, VerdictFail, ReasonOOMKilledDuringWindow},
		{"exited during window", GateObservation{Running: false, ExitCode: 1, ConsecutivePasses: 3, SinceStart: 3 * time.Second}, VerdictFail, ReasonExitedDuringWindow},
		{"restart count grew", GateObservation{Running: true, RestartCount: 1, ConsecutivePasses: 3, SinceStart: time.Minute}, VerdictFail, ReasonRestartedDuringWindow},
		{"started at moved", GateObservation{Running: true, StartedAt: start.Add(time.Second), BaselineStartedAt: start, ConsecutivePasses: 3, SinceStart: time.Minute}, VerdictFail, ReasonRestartedDuringWindow},
		{"unknown started at is not a restart", GateObservation{Running: true, BaselineStartedAt: start, ConsecutivePasses: 3, SinceStart: time.Minute}, VerdictPass, ""},
		{"budget exhausted while pending", GateObservation{Running: true, ConsecutivePasses: 1, SinceStart: time.Minute, BudgetExhausted: true}, VerdictFail, ReasonGateTimeout},
		{"budget exhausted but already passing", GateObservation{Running: true, ConsecutivePasses: 3, SinceStart: time.Minute, BudgetExhausted: true}, VerdictPass, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := EvaluateGate(tt.obs, cfg)
			if got != tt.want || reason != tt.wantReason {
				t.Errorf("EvaluateGate() = (%q, %q), want (%q, %q)", got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestEvaluateGate_ZeroThresholdMeansOne(t *testing.T) {
	got, _ := EvaluateGate(GateObservation{Running: true, ConsecutivePasses: 1}, Effective{})
	if got != VerdictPass {
		t.Errorf("EvaluateGate() = %q, want pass with a zero threshold and window", got)
	}
}

func TestEvaluateWatch(t *testing.T) {
	cfg := Effective{WatchWindow: 5 * time.Minute, WatchMaxRestarts: 2, WatchErrorRate: 0.2, WatchMinRequests: 20}
	tests := []struct {
		name       string
		obs        WatchObservation
		want       Verdict
		wantReason string
	}{
		{"quiet and in window", WatchObservation{SinceCutover: time.Minute}, VerdictPending, ""},
		{"window cleared", WatchObservation{SinceCutover: 5 * time.Minute}, VerdictPass, ""},
		{"oom", WatchObservation{SinceCutover: time.Minute, OOMKilled: true}, VerdictFail, ReasonWatchOOMKilled},
		{"two restarts", WatchObservation{Restarts: 2}, VerdictFail, ReasonWatchCrashloop},
		{"one restart plus currently exited", WatchObservation{Restarts: 1, Exited: true}, VerdictFail, ReasonWatchCrashloop},
		{"one restart only", WatchObservation{Restarts: 1}, VerdictPending, ""},
		{"error rate over limit", WatchObservation{Requests: 100, Errors5xx: 25}, VerdictFail, ReasonWatchErrorRate},
		{"error rate under limit", WatchObservation{Requests: 100, Errors5xx: 10}, VerdictPending, ""},
		{"too few requests to judge", WatchObservation{Requests: 10, Errors5xx: 10}, VerdictPending, ""},
		{"errors still trip after window", WatchObservation{SinceCutover: 10 * time.Minute, Requests: 50, Errors5xx: 50}, VerdictFail, ReasonWatchErrorRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason, _ := EvaluateWatch(tt.obs, cfg)
			if got != tt.want || reason != tt.wantReason {
				t.Errorf("EvaluateWatch() = (%q, %q), want (%q, %q)", got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestEvaluateWatch_SingleExitWithLimitOne(t *testing.T) {
	_, reason, _ := EvaluateWatch(WatchObservation{Exited: true}, Effective{WatchMaxRestarts: 1, WatchWindow: time.Minute})
	if reason != ReasonWatchExited {
		t.Errorf("reason = %q, want %q", reason, ReasonWatchExited)
	}
}

func TestDefaultsFromEnv(t *testing.T) {
	env := map[string]string{
		EnvSuccessThreshold: "4",
		EnvStableWindow:     "30s",
		EnvWatchErrorRate:   "0.5",
		EnvAutoRollback:     "false",
		EnvWatchMaxRestarts: "zero",
		EnvDrainTimeout:     "-1s",
	}
	got, warnings := DefaultsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got.SuccessThreshold != 4 || got.StableWindow != 30*time.Second || got.WatchErrorRate != 0.5 || got.AutoRollback {
		t.Errorf("DefaultsFromEnv() = %+v, want the env values applied", got)
	}
	if got.WatchMaxRestarts != BuiltinDefaults().WatchMaxRestarts || got.DrainTimeout != BuiltinDefaults().DrainTimeout {
		t.Errorf("invalid values were applied: %+v", got)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2", warnings)
	}
}

func TestResolveAndValidate(t *testing.T) {
	three, zero := 3, 0
	window := time.Minute
	tooLong := time.Hour
	rate := 1.5
	off := false
	got := Resolve(BuiltinDefaults(), store.DeploySafetySettings{SuccessThreshold: &three, StableWindow: &window, AutoRollback: &off})
	if got.SuccessThreshold != 3 || got.StableWindow != time.Minute || got.AutoRollback || got.DrainTimeout != BuiltinDefaults().DrainTimeout {
		t.Errorf("Resolve() = %+v", got)
	}
	tests := []struct {
		name    string
		s       store.DeploySafetySettings
		wantErr bool
	}{
		{"empty", store.DeploySafetySettings{}, false},
		{"valid", store.DeploySafetySettings{SuccessThreshold: &three, StableWindow: &window}, false},
		{"zero threshold", store.DeploySafetySettings{SuccessThreshold: &zero}, true},
		{"window too long", store.DeploySafetySettings{WatchWindow: &tooLong}, true},
		{"rate over one", store.DeploySafetySettings{WatchErrorRate: &rate}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
