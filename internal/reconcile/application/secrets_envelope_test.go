package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Fake-runtime twin of the live secret-env test: real store, key and manager, no Docker.
func TestController_Reconcile_EnvelopeSecretReachesContainerEnvOnly(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(db, mk)

	const name = "levelrail-test-secret-env"
	const plaintext = "sk-real-secret-abc123" //nolint:gosec // fixture, the test proves it never reaches the store verbatim
	if err := manager.SetValue(ctx, name, "API_KEY", plaintext); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	desired := store.DesiredService{
		Name: name, Image: "nginx:alpine", Port: 80,
		Env:       map[string]string{"NODE_ENV": "production"},
		SecretEnv: []store.SecretEnvRef{{Name: "API_KEY"}},
	}
	if err := db.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	rt := newFakeRuntime(0)
	result, err := New(name, db, rt, WithSecretResolver(manager)).Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile() error = %v, result = %+v", err, result)
	}
	if got := rt.lastCreateEnv["API_KEY"]; got != plaintext {
		t.Errorf("container env API_KEY = %q, want the decrypted secret", got)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal preserved", got)
	}
	stored, err := db.GetSecretValue(ctx, name, "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(stored) == plaintext {
		t.Fatal("stored secret equals the plaintext, encryption did not happen")
	}
}
