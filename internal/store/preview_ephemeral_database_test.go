package store

import (
	"context"
	"errors"
	"testing"
)

// seedPreviewEnvironmentForEphemeralDB inserts a minimal, valid parent
// preview_environments row: preview_ephemeral_databases.preview_environment_id
// references it, so every test in this file needs one to exist first.
func seedPreviewEnvironmentForEphemeralDB(t *testing.T, db *DB) PreviewEnvironment {
	t.Helper()
	p := testPreviewEnvironment()
	if err := db.SavePreviewEnvironment(context.Background(), p); err != nil {
		t.Fatalf("seed preview environment: %v", err)
	}
	return p
}

func testPreviewEphemeralDatabase(previewEnvironmentID string) PreviewEphemeralDatabase {
	return PreviewEphemeralDatabase{
		ID: "pedb_test1", PreviewEnvironmentID: previewEnvironmentID,
		DatabaseName: "web-pr-42-db-main", SourceKey: "main",
		Engine: "postgres", Version: "16", Status: PreviewEphemeralDatabaseStatusProvisioned,
		CreatedAt: "2026-08-20T00:00:00Z", UpdatedAt: "2026-08-20T00:00:00Z",
	}
}

func TestSaveAndGetPreviewEphemeralDatabaseByPreviewAndKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForEphemeralDB(t, db)
	want := testPreviewEphemeralDatabase(preview.ID)

	if err := db.SavePreviewEphemeralDatabase(ctx, want); err != nil {
		t.Fatalf("SavePreviewEphemeralDatabase() error = %v", err)
	}

	got, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(ctx, preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	if *got != want {
		t.Errorf("GetPreviewEphemeralDatabaseByPreviewAndKey() = %+v, want %+v", *got, want)
	}
}

func TestGetPreviewEphemeralDatabaseByPreviewAndKey_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), "prev_ghost", "main")
	if !errors.Is(err, ErrPreviewEphemeralDatabaseNotFound) {
		t.Fatalf("error = %v, want ErrPreviewEphemeralDatabaseNotFound", err)
	}
}

func TestListPreviewEphemeralDatabasesByPreview(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForEphemeralDB(t, db)

	main := testPreviewEphemeralDatabase(preview.ID)
	cache := testPreviewEphemeralDatabase(preview.ID)
	cache.ID, cache.DatabaseName, cache.SourceKey, cache.Engine = "pedb_test2", "web-pr-42-db-cache", "cache", "redis"
	if err := db.SavePreviewEphemeralDatabase(ctx, main); err != nil {
		t.Fatalf("save main: %v", err)
	}
	if err := db.SavePreviewEphemeralDatabase(ctx, cache); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	got, err := db.ListPreviewEphemeralDatabasesByPreview(ctx, preview.ID)
	if err != nil {
		t.Fatalf("ListPreviewEphemeralDatabasesByPreview() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].SourceKey != "cache" || got[1].SourceKey != "main" {
		t.Errorf("got source keys = [%s, %s], want [cache, main] (alphabetical)", got[0].SourceKey, got[1].SourceKey)
	}
}

func TestListPreviewEphemeralDatabasesByPreview_Empty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListPreviewEphemeralDatabasesByPreview(context.Background(), "prev_ghost")
	if err != nil {
		t.Fatalf("ListPreviewEphemeralDatabasesByPreview() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestUpdatePreviewEphemeralDatabaseStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForEphemeralDB(t, db)
	seeded := testPreviewEphemeralDatabase(preview.ID)
	if err := db.SavePreviewEphemeralDatabase(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewEphemeralDatabase() error = %v", err)
	}

	if err := db.UpdatePreviewEphemeralDatabaseStatus(ctx, seeded.ID, PreviewEphemeralDatabaseStatusTeardownFailed, "engine unavailable", "2026-08-21T00:00:00Z"); err != nil {
		t.Fatalf("UpdatePreviewEphemeralDatabaseStatus() error = %v", err)
	}

	got, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(ctx, preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	if got.Status != PreviewEphemeralDatabaseStatusTeardownFailed || got.StatusReason != "engine unavailable" {
		t.Errorf("got = %+v, want status=teardown_failed reason=\"engine unavailable\"", got)
	}
	if got.UpdatedAt != "2026-08-21T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-08-21T00:00:00Z", got.UpdatedAt)
	}
}

func TestUpdatePreviewEphemeralDatabaseStatus_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdatePreviewEphemeralDatabaseStatus(context.Background(), "pedb_ghost", PreviewEphemeralDatabaseStatusTeardownFailed, "boom", "2026-08-21T00:00:00Z")
	if !errors.Is(err, ErrPreviewEphemeralDatabaseNotFound) {
		t.Fatalf("error = %v, want ErrPreviewEphemeralDatabaseNotFound", err)
	}
}

func TestDeletePreviewEphemeralDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	preview := seedPreviewEnvironmentForEphemeralDB(t, db)
	seeded := testPreviewEphemeralDatabase(preview.ID)
	if err := db.SavePreviewEphemeralDatabase(ctx, seeded); err != nil {
		t.Fatalf("SavePreviewEphemeralDatabase() error = %v", err)
	}

	if err := db.DeletePreviewEphemeralDatabase(ctx, seeded.ID); err != nil {
		t.Fatalf("DeletePreviewEphemeralDatabase() error = %v", err)
	}

	_, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(ctx, preview.ID, "main")
	if !errors.Is(err, ErrPreviewEphemeralDatabaseNotFound) {
		t.Fatalf("error = %v, want ErrPreviewEphemeralDatabaseNotFound after delete", err)
	}
}

func TestDeletePreviewEphemeralDatabase_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeletePreviewEphemeralDatabase(context.Background(), "pedb_ghost")
	if !errors.Is(err, ErrPreviewEphemeralDatabaseNotFound) {
		t.Fatalf("error = %v, want ErrPreviewEphemeralDatabaseNotFound", err)
	}
}

func TestNewPreviewEphemeralDatabaseID_HasPrefix(t *testing.T) {
	id, err := NewPreviewEphemeralDatabaseID()
	if err != nil {
		t.Fatalf("NewPreviewEphemeralDatabaseID() error = %v", err)
	}
	if len(id) <= len(previewEphemeralDatabaseIDPrefix) || id[:len(previewEphemeralDatabaseIDPrefix)] != previewEphemeralDatabaseIDPrefix {
		t.Errorf("id = %q, want prefix %q", id, previewEphemeralDatabaseIDPrefix)
	}
}
