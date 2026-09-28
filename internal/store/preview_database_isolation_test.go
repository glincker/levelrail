package store

import (
	"context"
	"errors"
	"testing"
)

func seedPreviewEnvironmentForDBIsolation(t *testing.T, db *DB) PreviewEnvironment {
	t.Helper()
	p := testPreviewEnvironment()
	if err := db.SavePreviewEnvironment(context.Background(), p); err != nil {
		t.Fatalf("seed preview environment: %v", err)
	}
	return p
}

func testPreviewDatabaseIsolation(previewEnvironmentID string) PreviewDatabaseIsolation {
	return PreviewDatabaseIsolation{
		ID: "pdbi_test1", PreviewEnvironmentID: previewEnvironmentID,
		DatabaseName: "main", SourceKey: "main", RoleName: "pv_web_pr_42_main", SecretEnvKey: "password",
		Status:    PreviewDatabaseIsolationStatusProvisioned,
		CreatedAt: "2026-09-20T00:00:00Z", UpdatedAt: "2026-09-20T00:00:00Z",
	}
}

func TestSaveAndGetPreviewDatabaseIsolationByPreviewAndKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForDBIsolation(t, db)
	want := testPreviewDatabaseIsolation(preview.ID)

	if err := db.SavePreviewDatabaseIsolation(ctx, want); err != nil {
		t.Fatalf("SavePreviewDatabaseIsolation() error = %v", err)
	}

	got, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(ctx, preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
	}
	if *got != want {
		t.Errorf("GetPreviewDatabaseIsolationByPreviewAndKey() = %+v, want %+v", *got, want)
	}
}

func TestGetPreviewDatabaseIsolationByPreviewAndKey_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), "prev_ghost", "main")
	if !errors.Is(err, ErrPreviewDatabaseIsolationNotFound) {
		t.Fatalf("error = %v, want ErrPreviewDatabaseIsolationNotFound", err)
	}
}

func TestListPreviewDatabaseIsolationsByPreview(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForDBIsolation(t, db)

	main := testPreviewDatabaseIsolation(preview.ID)
	cache := testPreviewDatabaseIsolation(preview.ID)
	cache.ID, cache.DatabaseName, cache.SourceKey, cache.RoleName = "pdbi_test2", "cache", "cache", "pv_web_pr_42_cache"
	if err := db.SavePreviewDatabaseIsolation(ctx, main); err != nil {
		t.Fatalf("save main: %v", err)
	}
	if err := db.SavePreviewDatabaseIsolation(ctx, cache); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	got, err := db.ListPreviewDatabaseIsolationsByPreview(ctx, preview.ID)
	if err != nil {
		t.Fatalf("ListPreviewDatabaseIsolationsByPreview() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].SourceKey != "cache" || got[1].SourceKey != "main" {
		t.Errorf("got source keys = [%s, %s], want [cache, main] (alphabetical)", got[0].SourceKey, got[1].SourceKey)
	}
}

func TestListPreviewDatabaseIsolationsByPreview_Empty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListPreviewDatabaseIsolationsByPreview(context.Background(), "prev_ghost")
	if err != nil {
		t.Fatalf("ListPreviewDatabaseIsolationsByPreview() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestUpdatePreviewDatabaseIsolationStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForDBIsolation(t, db)
	seeded := testPreviewDatabaseIsolation(preview.ID)
	if err := db.SavePreviewDatabaseIsolation(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewDatabaseIsolation() error = %v", err)
	}

	if err := db.UpdatePreviewDatabaseIsolationStatus(ctx, seeded.ID, PreviewDatabaseIsolationStatusTeardownFailed, "container unreachable", "2026-09-21T00:00:00Z"); err != nil {
		t.Fatalf("UpdatePreviewDatabaseIsolationStatus() error = %v", err)
	}

	got, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(ctx, preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
	}
	if got.Status != PreviewDatabaseIsolationStatusTeardownFailed || got.StatusReason != "container unreachable" {
		t.Errorf("got = %+v, want status=teardown_failed reason=\"container unreachable\"", got)
	}
	if got.UpdatedAt != "2026-09-21T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-09-21T00:00:00Z", got.UpdatedAt)
	}
}

func TestUpdatePreviewDatabaseIsolationStatus_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdatePreviewDatabaseIsolationStatus(context.Background(), "pdbi_ghost", PreviewDatabaseIsolationStatusTeardownFailed, "boom", "2026-09-21T00:00:00Z")
	if !errors.Is(err, ErrPreviewDatabaseIsolationNotFound) {
		t.Fatalf("error = %v, want ErrPreviewDatabaseIsolationNotFound", err)
	}
}

func TestDeletePreviewDatabaseIsolation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForDBIsolation(t, db)
	seeded := testPreviewDatabaseIsolation(preview.ID)
	if err := db.SavePreviewDatabaseIsolation(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewDatabaseIsolation() error = %v", err)
	}

	if err := db.DeletePreviewDatabaseIsolation(ctx, seeded.ID); err != nil {
		t.Fatalf("DeletePreviewDatabaseIsolation() error = %v", err)
	}

	_, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(ctx, preview.ID, "main")
	if !errors.Is(err, ErrPreviewDatabaseIsolationNotFound) {
		t.Fatalf("error = %v, want ErrPreviewDatabaseIsolationNotFound after delete", err)
	}
}

func TestDeletePreviewDatabaseIsolation_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeletePreviewDatabaseIsolation(context.Background(), "pdbi_ghost")
	if !errors.Is(err, ErrPreviewDatabaseIsolationNotFound) {
		t.Fatalf("error = %v, want ErrPreviewDatabaseIsolationNotFound", err)
	}
}

func TestNewPreviewDatabaseIsolationID_HasPrefix(t *testing.T) {
	id, err := NewPreviewDatabaseIsolationID()
	if err != nil {
		t.Fatalf("NewPreviewDatabaseIsolationID() error = %v", err)
	}
	if len(id) <= len(previewDatabaseIsolationIDPrefix) || id[:len(previewDatabaseIsolationIDPrefix)] != previewDatabaseIsolationIDPrefix {
		t.Errorf("id = %q, want prefix %q", id, previewDatabaseIsolationIDPrefix)
	}
}
