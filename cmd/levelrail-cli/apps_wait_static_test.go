package main

import (
	"testing"
	"time"
)

func TestComputeRolloutOutcome_StaticSite(t *testing.T) {
	fin := time.Now()
	tests := []struct {
		name  string
		a     deployAttemptResource
		conds []conditionResource
		want  string
	}{
		{"serving rollout state", deployAttemptResource{Status: "succeeded", FinishedAt: &fin, RolloutState: "serving"}, nil, "succeeded"},
		{"published condition", deployAttemptResource{Status: "succeeded", FinishedAt: &fin}, []conditionResource{{Reason: "StaticSitePublished", LastTransitionTime: fin.Add(time.Second)}}, "succeeded"},
		{"nothing observed yet", deployAttemptResource{Status: "succeeded", FinishedAt: &fin}, nil, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeRolloutOutcome(tt.a, tt.conds); got.state != tt.want {
				t.Errorf("state = %q, want %q", got.state, tt.want)
			}
		})
	}
}
