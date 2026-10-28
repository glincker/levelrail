package deploy

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRestoreSnapshot_KeepsRunToCompletion(t *testing.T) {
	current := store.DesiredService{Name: "migrate", Port: 80, Health: &store.ServiceHealth{Restart: store.RestartPolicyOnFailure}}
	old := store.DeployAttemptSnapshot{Port: 81}
	if got := RestoreSnapshot(current, old); got.Health == nil || got.Health.Restart != store.RestartPolicyOnFailure {
		t.Errorf("snapshot without restart: health = %+v, want current restart kept", got.Health)
	}
	withRestart := store.DeployAttemptSnapshot{Port: 81, Health: &store.ServiceHealth{Restart: store.RestartPolicyNo}}
	if got := RestoreSnapshot(current, withRestart); got.Health == nil || got.Health.Restart != store.RestartPolicyNo {
		t.Errorf("snapshot with restart: health = %+v, want snapshot restart", got.Health)
	}
}
