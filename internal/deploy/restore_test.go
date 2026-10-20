package deploy

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRestoreSnapshot(t *testing.T) {
	current := store.DesiredService{
		Name: "web", Image: "app:v2", Port: 9090, Replicas: 3, Strategy: "rolling",
		Env:       map[string]string{"MODE": "new", "ADDED_LATER": "x", "KEEP": "same"},
		SecretEnv: []store.SecretEnvRef{{Name: "API_KEY"}},
		Domains:   []string{"app.example.com"},
		Health:    &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/new"}},
	}
	snap := store.DeployAttemptSnapshot{
		Env: []store.DeployAttemptEnvKey{
			{Key: "MODE", Kind: store.DeployAttemptEnvKindLiteral, Value: "old"},
			{Key: "KEEP", Kind: store.DeployAttemptEnvKindLiteral, Value: "same"},
			{Key: "API_KEY", Kind: store.DeployAttemptEnvKindSecret},
		},
		Port: 8080, Replicas: 2, Strategy: "blue-green",
		Health:  &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/old"}},
		Domains: []string{"old.example.com"},
	}

	got := RestoreSnapshot(current, snap)
	wantEnv := map[string]string{"MODE": "old", "KEEP": "same"}
	if !reflect.DeepEqual(got.Env, wantEnv) {
		t.Errorf("Env = %v, want %v", got.Env, wantEnv)
	}
	if got.Port != 8080 || got.Replicas != 2 || got.Strategy != "blue-green" || got.Health.Readiness.Path != "/old" {
		t.Errorf("restored config = port %d replicas %d strategy %q health %+v", got.Port, got.Replicas, got.Strategy, got.Health.Readiness)
	}
	if !reflect.DeepEqual(got.Domains, current.Domains) || len(got.SecretEnv) != 1 {
		t.Errorf("domains or secret refs changed: %+v %+v", got.Domains, got.SecretEnv)
	}
	if current.Env["MODE"] != "new" {
		t.Error("RestoreSnapshot mutated its input")
	}
}

func TestRestoreSnapshot_EmptySnapshotIsNoop(t *testing.T) {
	current := store.DesiredService{Name: "web", Env: map[string]string{"A": "1"}, Port: 80}
	if got := RestoreSnapshot(current, store.DeployAttemptSnapshot{}); !reflect.DeepEqual(got, current) {
		t.Errorf("RestoreSnapshot(empty) = %+v, want unchanged", got)
	}
}

func TestRollbackRef(t *testing.T) {
	pin := &store.RollbackPin{PinRef: "x-rollback/web:dep_1"}
	tests := []struct {
		name     string
		target   store.DeployAttempt
		pin      *store.RollbackPin
		wantRef  string
		wantSafe bool
	}{
		{"registry digest wins", store.DeployAttempt{Image: "reg/app:v1", ImageDigest: "sha256:abc", DigestReason: store.DigestReasonResolved}, pin, "reg/app:v1@sha256:abc", true},
		{"local build uses pin", store.DeployAttempt{Image: "app:v1", ImageDigest: "sha256:abc", DigestReason: store.DigestReasonLocalBuild}, pin, pin.PinRef, true},
		{"nothing pinned falls back to tag", store.DeployAttempt{Image: "app:v1"}, nil, "app:v1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, safe := RollbackRef(tt.target, tt.pin)
			if ref != tt.wantRef || safe != tt.wantSafe {
				t.Errorf("RollbackRef() = (%q, %v), want (%q, %v)", ref, safe, tt.wantRef, tt.wantSafe)
			}
		})
	}
}
