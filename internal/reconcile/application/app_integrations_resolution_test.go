package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_ResolveEnv_AppIntegration proves that an attached
// internal/integrations catalog entry actually resolves its declared env
// vars end to end through the real internal/store app_integrations
// table and internal/secrets envelope encryption, not a hand-written
// fake standing in for either layer: a stored field value wins over the
// catalog's own Default, and a field with no stored value falls back to
// its Default.
func TestController_ResolveEnv_AppIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)

	now := time.Now().UTC()
	if err := db.SaveAppIntegration(ctx, store.AppIntegration{ID: "appint_1", ServiceName: "web", IntegrationKey: "posthog", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	// Only the required field is set; NEXT_PUBLIC_POSTHOG_HOST is left
	// unset and must fall back to its catalog Default.
	namespace := store.AppIntegrationSecretsKey("web", "posthog")
	if err := secretsManager.SetValue(ctx, namespace, "NEXT_PUBLIC_POSTHOG_KEY", "phc_test123"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	c := New("web", &fakeStore{}, newFakeRuntime(0),
		WithAppIntegrations(db),
		WithSecretResolver(secretsManager),
	)

	desired := &store.DesiredService{
		Name:  "web",
		Image: "img:v1",
		Port:  80,
		Env:   map[string]string{"APP_OWN": "app-value"},
	}

	got, err := c.resolveEnv(ctx, desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}

	want := map[string]string{
		"NEXT_PUBLIC_POSTHOG_KEY":  "phc_test123",
		"NEXT_PUBLIC_POSTHOG_HOST": "https://us.i.posthog.com",
		"APP_OWN":                  "app-value",
	}
	if len(got) != len(want) {
		t.Errorf("resolveEnv() = %v, want %v", got, want)
	}
	for k, wantV := range want {
		if gotV := got[k]; gotV != wantV {
			t.Errorf("resolveEnv()[%q] = %q, want %q", k, gotV, wantV)
		}
	}
}

// TestController_ResolveEnv_AppIntegration_AppOwnOverrides pins
// resolveEnv's documented precedence: the app's own Env always wins over
// a same-named integration-injected value, even when the integration
// declares that exact env var name.
func TestController_ResolveEnv_AppIntegration_AppOwnOverrides(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)

	now := time.Now().UTC()
	if err := db.SaveAppIntegration(ctx, store.AppIntegration{ID: "appint_1", ServiceName: "web", IntegrationKey: "sentry", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	namespace := store.AppIntegrationSecretsKey("web", "sentry")
	if err := secretsManager.SetValue(ctx, namespace, "SENTRY_DSN", "https://integration@o0.ingest.sentry.io/0"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	c := New("web", &fakeStore{}, newFakeRuntime(0),
		WithAppIntegrations(db),
		WithSecretResolver(secretsManager),
	)

	desired := &store.DesiredService{
		Name:  "web",
		Image: "img:v1",
		Port:  80,
		Env:   map[string]string{"SENTRY_DSN": "https://app-own@o0.ingest.sentry.io/0"},
	}

	got, err := c.resolveEnv(ctx, desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}
	if got["SENTRY_DSN"] != "https://app-own@o0.ingest.sentry.io/0" {
		t.Errorf("resolveEnv()[SENTRY_DSN] = %q, want the app's own value to win over the attached integration's", got["SENTRY_DSN"])
	}
}

// TestController_ResolveEnv_AppIntegration_UnknownCatalogKeySkipped
// proves that an attachment whose catalog entry no longer exists (e.g.
// removed from the catalog after the app attached it) is silently
// skipped rather than failing the reconcile.
func TestController_ResolveEnv_AppIntegration_UnknownCatalogKeySkipped(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	now := time.Now().UTC()
	if err := db.SaveAppIntegration(ctx, store.AppIntegration{ID: "appint_1", ServiceName: "web", IntegrationKey: "not-a-real-tool", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}

	c := New("web", &fakeStore{}, newFakeRuntime(0), WithAppIntegrations(db))

	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}

	got, err := c.resolveEnv(ctx, desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("resolveEnv() = %v, want empty (unknown catalog key skipped)", got)
	}
}
