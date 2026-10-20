package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedFeatureFlagService(t *testing.T, db *DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed service %q: %v", name, err)
	}
}

func newTestFeatureFlag(id, key, service string) FeatureFlag {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	return FeatureFlag{
		ID:                id,
		Key:               key,
		Name:              "New checkout",
		Description:       "Gates the redesigned checkout flow",
		ServiceName:       service,
		Enabled:           true,
		RolloutPercentage: 100,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func TestSaveAndGetFeatureFlag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")

	want := newTestFeatureFlag("ff_1", "new-checkout", "web")
	if err := db.SaveFeatureFlag(ctx, want); err != nil {
		t.Fatalf("SaveFeatureFlag() error = %v", err)
	}

	got, err := db.GetFeatureFlag(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetFeatureFlag() error = %v", err)
	}
	if got.ID != want.ID || got.Key != want.Key || got.ServiceName != want.ServiceName ||
		got.Enabled != want.Enabled || got.RolloutPercentage != want.RolloutPercentage {
		t.Errorf("GetFeatureFlag() = %+v, want %+v", got, want)
	}
}

func TestGetFeatureFlag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetFeatureFlag(ctx, "ff_missing")
	if !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("GetFeatureFlag() error = %v, want ErrFeatureFlagNotFound", err)
	}
}

func TestGetFeatureFlagByKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")

	want := newTestFeatureFlag("ff_1", "new-checkout", "web")
	if err := db.SaveFeatureFlag(ctx, want); err != nil {
		t.Fatalf("SaveFeatureFlag() error = %v", err)
	}

	got, err := db.GetFeatureFlagByKey(ctx, "new-checkout")
	if err != nil {
		t.Fatalf("GetFeatureFlagByKey() error = %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("GetFeatureFlagByKey() ID = %q, want %q", got.ID, want.ID)
	}
}

func TestGetFeatureFlagByKey_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetFeatureFlagByKey(ctx, "missing-key")
	if !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("GetFeatureFlagByKey() error = %v, want ErrFeatureFlagNotFound", err)
	}
}

func TestSaveFeatureFlag_DuplicateKeyRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")
	seedFeatureFlagService(t, db, "worker")

	if err := db.SaveFeatureFlag(ctx, newTestFeatureFlag("ff_1", "shared-key", "web")); err != nil {
		t.Fatalf("SaveFeatureFlag(ff_1) error = %v", err)
	}
	if err := db.SaveFeatureFlag(ctx, newTestFeatureFlag("ff_2", "shared-key", "worker")); err == nil {
		t.Fatal("SaveFeatureFlag() with a duplicate key = nil error, want a UNIQUE constraint failure")
	}
}

func TestListFeatureFlagsForService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")
	seedFeatureFlagService(t, db, "worker")

	a := newTestFeatureFlag("ff_a", "flag-a", "web")
	a.CreatedAt = time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	b := newTestFeatureFlag("ff_b", "flag-b", "web")
	b.CreatedAt = time.Date(2026, 8, 30, 0, 0, 1, 0, time.UTC)
	other := newTestFeatureFlag("ff_c", "flag-c", "worker")

	for _, f := range []FeatureFlag{b, a, other} {
		if err := db.SaveFeatureFlag(ctx, f); err != nil {
			t.Fatalf("SaveFeatureFlag(%q) error = %v", f.ID, err)
		}
	}

	got, err := db.ListFeatureFlagsForService(ctx, "web")
	if err != nil {
		t.Fatalf("ListFeatureFlagsForService() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "ff_a" || got[1].ID != "ff_b" {
		t.Fatalf("ListFeatureFlagsForService() = %+v, want [ff_a, ff_b] in creation order", got)
	}
}

func TestUpdateFeatureFlag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")

	f := newTestFeatureFlag("ff_1", "new-checkout", "web")
	if err := db.SaveFeatureFlag(ctx, f); err != nil {
		t.Fatalf("SaveFeatureFlag() error = %v", err)
	}

	updatedAt := f.UpdatedAt.Add(time.Hour)
	if err := db.UpdateFeatureFlag(ctx, f.ID, "New checkout v2", "updated desc", false, 25, updatedAt); err != nil {
		t.Fatalf("UpdateFeatureFlag() error = %v", err)
	}

	got, err := db.GetFeatureFlag(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFeatureFlag() error = %v", err)
	}
	if got.Name != "New checkout v2" || got.Description != "updated desc" || got.Enabled || got.RolloutPercentage != 25 {
		t.Errorf("GetFeatureFlag() after update = %+v, unexpected", got)
	}
	if got.Key != "new-checkout" {
		t.Errorf("GetFeatureFlag() Key after update = %q, want unchanged %q", got.Key, "new-checkout")
	}
}

func TestUpdateFeatureFlag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateFeatureFlag(ctx, "ff_missing", "name", "desc", true, 50, time.Now())
	if !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("UpdateFeatureFlag() error = %v, want ErrFeatureFlagNotFound", err)
	}
}

func TestDeleteFeatureFlag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedFeatureFlagService(t, db, "web")

	f := newTestFeatureFlag("ff_1", "new-checkout", "web")
	if err := db.SaveFeatureFlag(ctx, f); err != nil {
		t.Fatalf("SaveFeatureFlag() error = %v", err)
	}

	if err := db.DeleteFeatureFlag(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFeatureFlag() error = %v", err)
	}

	if _, err := db.GetFeatureFlag(ctx, f.ID); !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("GetFeatureFlag() after delete error = %v, want ErrFeatureFlagNotFound", err)
	}
}

func TestDeleteFeatureFlag_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteFeatureFlag(ctx, "ff_missing")
	if !errors.Is(err, ErrFeatureFlagNotFound) {
		t.Fatalf("DeleteFeatureFlag() error = %v, want ErrFeatureFlagNotFound", err)
	}
}
