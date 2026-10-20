package store

import (
	"context"
	"testing"
)

// TestGetUpdateSettings_Default proves migrations/0258's own seeded row
// round-trips into stable/disabled: an operator who never visits the
// settings page must see identical behavior to before this table
// existed (GET /api/v1/updates always compared against stable).
func TestGetUpdateSettings_Default(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetUpdateSettings(ctx)
	if err != nil {
		t.Fatalf("GetUpdateSettings() error = %v", err)
	}
	want := UpdateSettings{Channel: "stable", AutoUpdateEnabled: false}
	if got != want {
		t.Errorf("GetUpdateSettings() = %+v, want %+v", got, want)
	}
}

func TestUpdateAndGetUpdateSettings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := UpdateSettings{Channel: "beta", AutoUpdateEnabled: true}
	if err := db.UpdateUpdateSettings(ctx, want); err != nil {
		t.Fatalf("UpdateUpdateSettings() error = %v", err)
	}

	got, err := db.GetUpdateSettings(ctx)
	if err != nil {
		t.Fatalf("GetUpdateSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetUpdateSettings() = %+v, want %+v", got, want)
	}
}

func TestUpdateUpdateSettings_MultipleUpdatesStaySingleRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateUpdateSettings(ctx, UpdateSettings{Channel: "beta", AutoUpdateEnabled: true}); err != nil {
		t.Fatalf("UpdateUpdateSettings(1) error = %v", err)
	}
	if err := db.UpdateUpdateSettings(ctx, UpdateSettings{Channel: "edge", AutoUpdateEnabled: false}); err != nil {
		t.Fatalf("UpdateUpdateSettings(2) error = %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM update_settings`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("update_settings row count = %d, want 1 (singleton table)", count)
	}

	got, err := db.GetUpdateSettings(ctx)
	if err != nil {
		t.Fatalf("GetUpdateSettings() error = %v", err)
	}
	want := UpdateSettings{Channel: "edge", AutoUpdateEnabled: false}
	if got != want {
		t.Errorf("GetUpdateSettings() = %+v, want the latest update %+v", got, want)
	}
}
