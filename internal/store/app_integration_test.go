package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestAppIntegration(id, serviceName, key string) AppIntegration {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	return AppIntegration{ID: id, ServiceName: serviceName, IntegrationKey: key, CreatedAt: now, UpdatedAt: now}
}

func TestSaveAndGetAppIntegration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	want := newTestAppIntegration("appint_1", "web", "sentry")
	if err := db.SaveAppIntegration(ctx, want); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}

	got, err := db.GetAppIntegration(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetAppIntegration() error = %v", err)
	}
	if got.ServiceName != want.ServiceName || got.IntegrationKey != want.IntegrationKey {
		t.Errorf("GetAppIntegration() = %+v, want %+v", got, want)
	}
}

func TestGetAppIntegration_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetAppIntegration(ctx, "appint_missing")
	if !errors.Is(err, ErrAppIntegrationNotFound) {
		t.Fatalf("GetAppIntegration() error = %v, want ErrAppIntegrationNotFound", err)
	}
}

func TestSaveAppIntegration_AlreadyAttached(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	if err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_1", "web", "sentry")); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_2", "web", "sentry"))
	if !errors.Is(err, ErrAppIntegrationAlreadyAttached) {
		t.Fatalf("SaveAppIntegration() error = %v, want ErrAppIntegrationAlreadyAttached", err)
	}
}

func TestListAppIntegrationsForService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")
	seedTagService(t, db, "api")

	if err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_1", "web", "sentry")); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	if err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_2", "web", "posthog")); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	if err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_3", "api", "sentry")); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}

	got, err := db.ListAppIntegrationsForService(ctx, "web")
	if err != nil {
		t.Fatalf("ListAppIntegrationsForService() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAppIntegrationsForService() returned %d rows, want 2", len(got))
	}
	if got[0].IntegrationKey != "sentry" || got[1].IntegrationKey != "posthog" {
		t.Errorf("ListAppIntegrationsForService() = %+v, want sentry then posthog (creation order)", got)
	}
}

func TestDeleteAppIntegration(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")

	if err := db.SaveAppIntegration(ctx, newTestAppIntegration("appint_1", "web", "sentry")); err != nil {
		t.Fatalf("SaveAppIntegration() error = %v", err)
	}
	if err := db.DeleteAppIntegration(ctx, "appint_1"); err != nil {
		t.Fatalf("DeleteAppIntegration() error = %v", err)
	}
	if _, err := db.GetAppIntegration(ctx, "appint_1"); !errors.Is(err, ErrAppIntegrationNotFound) {
		t.Fatalf("GetAppIntegration() after delete error = %v, want ErrAppIntegrationNotFound", err)
	}
}

func TestDeleteAppIntegration_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteAppIntegration(ctx, "appint_missing")
	if !errors.Is(err, ErrAppIntegrationNotFound) {
		t.Fatalf("DeleteAppIntegration() error = %v, want ErrAppIntegrationNotFound", err)
	}
}

func TestAppIntegrationSecretsKey(t *testing.T) {
	got := AppIntegrationSecretsKey("web", "sentry")
	want := "app-integration/web/sentry"
	if got != want {
		t.Errorf("AppIntegrationSecretsKey() = %q, want %q", got, want)
	}
}
