package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPickServingImage(t *testing.T) {
	serving := store.RolloutStateServing
	tests := []struct {
		name     string
		attempts []store.DeployAttempt
		want     string
		wantOK   bool
	}{
		{"rolling out keeps old image", []store.DeployAttempt{
			{Image: "web:2", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:1", Status: store.DeployAttemptStatusSucceeded, RolloutState: serving},
		}, "web:1", true},
		{"new image serving", []store.DeployAttempt{
			{Image: "web:2", Status: store.DeployAttemptStatusSucceeded, RolloutState: serving},
			{Image: "web:1", Status: store.DeployAttemptStatusSucceeded, RolloutState: serving},
		}, "", false},
		{"first deploy has no fallback", []store.DeployAttempt{
			{Image: "web:1", Status: store.DeployAttemptStatusSucceeded},
		}, "", false},
		{"build failed attempt is not pending", []store.DeployAttempt{
			{Image: "web:2", Status: store.DeployAttemptStatusFailed},
			{Image: "web:1", RolloutState: serving},
		}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickServingImage(tc.attempts, tc.attempts[0].Image)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("got (%q,%v), want (%q,%v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestPresentedDeployStatus(t *testing.T) {
	tests := []struct{ stored, outcome, want string }{
		{"succeeded", OutcomeInProgress, StatusRollingOut},
		{"succeeded", OutcomeFailed, "failed"},
		{"succeeded", OutcomeHealthy, "succeeded"},
		{"succeeded", OutcomeSuperseded, "succeeded"},
		{"running", OutcomeInProgress, StatusBuilding},
		{"failed", OutcomeFailed, "failed"},
		{"canceled", OutcomeCanceled, "canceled"},
	}
	for _, tc := range tests {
		if got := presentedDeployStatus(tc.stored, tc.outcome); got != tc.want {
			t.Errorf("%s/%s: got %s want %s", tc.stored, tc.outcome, got, tc.want)
		}
	}
}
