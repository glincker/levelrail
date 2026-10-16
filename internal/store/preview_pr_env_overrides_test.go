package store

import (
	"context"
	"errors"
	"testing"
)

func TestPRPreviewEnvOverrides(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id1, err := db.SetPRPreviewEnvOverride(ctx, "web", 7, "A", "1", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	id2, err := db.SetPRPreviewEnvOverride(ctx, "web", 7, "A", "2", false)
	if err != nil || id2 != id1 {
		t.Fatalf("re-Set() = %q, %v, want the same id %q updated in place", id2, err, id1)
	}
	if _, err := db.SetPRPreviewEnvOverride(ctx, "web", 7, "S", "topsecret", true); err != nil {
		t.Fatalf("Set(secret) error = %v", err)
	}
	if _, err := db.SetPRPreviewEnvOverride(ctx, "web", 8, "A", "other", false); err != nil {
		t.Fatalf("Set(other pr) error = %v", err)
	}

	got, err := db.ListPRPreviewEnvOverrides(ctx, "web", 7)
	if err != nil || len(got) != 2 {
		t.Fatalf("List() = %+v, %v, want 2 rows for PR 7 only", got, err)
	}
	if got[0].Key != "A" || got[0].Value != "2" || got[1].Key != "S" || got[1].Value != "" || !got[1].Secret {
		t.Errorf("List() = %+v, want A=2 then secret S with an empty stored value", got)
	}

	if err := db.DeletePRPreviewEnvOverride(ctx, "web", 8, id1); !errors.Is(err, ErrPRPreviewEnvOverrideNotFound) {
		t.Errorf("Delete(wrong pr) error = %v, want not found", err)
	}
	if err := db.DeletePRPreviewEnvOverride(ctx, "web", 7, id1); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := db.DeleteAllPRPreviewEnvOverrides(ctx, "web", 7); err != nil {
			t.Errorf("DeleteAll() #%d error = %v, want idempotent", i+1, err)
		}
	}
	if left, _ := db.ListPRPreviewEnvOverrides(ctx, "web", 8); len(left) != 1 {
		t.Errorf("PR 8 rows = %d, want 1 untouched", len(left))
	}
}

func TestPreviewPendingVolumes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := db.AddPreviewPendingVolumes(ctx, "web-pr-1", []string{"b", "a"}); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}
	got, err := db.ListPreviewPendingVolumes(ctx, "web-pr-1")
	if err != nil || len(got) != 2 || got[0] != "a" {
		t.Fatalf("List() = %v, %v, want [a b]", got, err)
	}
	if err := db.ClearPreviewPendingVolumes(ctx, "web-pr-1"); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if got, _ := db.ListPreviewPendingVolumes(ctx, "web-pr-1"); len(got) != 0 {
		t.Errorf("List() after Clear = %v, want empty", got)
	}
}
