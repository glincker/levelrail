package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComputeDeployOutcome_StaticSite(t *testing.T) {
	done := time.Now()
	published := []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "StaticSitePublished", LastTransitionTime: done.Add(time.Second)}}
	tests := []struct {
		name  string
		a     store.DeployAttempt
		conds []reconcile.Condition
		want  string
	}{
		{"serving rollout state settles healthy", store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, RolloutState: store.RolloutStateServing, FinishedAt: &done}, nil, OutcomeHealthy},
		{"published condition alone settles healthy", store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &done}, published, OutcomeHealthy},
		{"no signal yet stays in progress", store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &done}, nil, OutcomeInProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeDeployOutcome(tt.a, true, tt.conds); got != tt.want {
				t.Errorf("outcome = %q, want %q", got, tt.want)
			}
		})
	}
}
