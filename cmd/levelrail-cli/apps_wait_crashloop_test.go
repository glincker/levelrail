package main

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

func TestComputeRolloutOutcome_CrashLoop(t *testing.T) {
	t.Setenv("APP_DEPLOY_ROLLOUT_CRASHLOOP_RESTARTS", "")
	fin := time.Now()
	loop := func(n int) conditionResource {
		return conditionResource{Reason: deploysafety.ReasonCrashLoopBackOff, Message: deploysafety.CrashLoopMessage("web-abc", n, 4*time.Second), LastTransitionTime: fin.Add(time.Second)}
	}
	pending := deployAttemptResource{Status: "succeeded", FinishedAt: &fin}
	serving := pending
	serving.RolloutState = "serving"
	tests := []struct {
		name  string
		a     deployAttemptResource
		conds []conditionResource
		want  string
	}{
		{"first deploy keeps crash looping", pending, []conditionResource{loop(3)}, "failed"},
		{"one restart is not yet a crash loop", pending, []conditionResource{loop(1)}, "pending"},
		{"crash then recovery", pending, []conditionResource{{Reason: "Deployed", LastTransitionTime: fin.Add(time.Minute)}}, "succeeded"},
		{"steady-state crash after success", serving, []conditionResource{loop(5)}, "succeeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeRolloutOutcome(tt.a, tt.conds); got.state != tt.want {
				t.Errorf("state = %q, want %q", got.state, tt.want)
			}
		})
	}
}
