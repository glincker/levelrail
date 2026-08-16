package store

import (
	"context"
	"testing"
)

func TestSetAndListProjectEnvVars(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, newTestProject()); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	want := map[string]string{"LOG_LEVEL": "info", "NODE_ENV": "production"}
	if err := db.SetProjectEnvVars(ctx, "proj_test1", want); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	got, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ListProjectEnvVars() = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListProjectEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestListProjectEnvVars_NoneSet_ReturnsEmptyNotNil(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, newTestProject()); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	got, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if got == nil {
		t.Error("ListProjectEnvVars() = nil, want an empty map")
	}
	if len(got) != 0 {
		t.Errorf("ListProjectEnvVars() = %+v, want empty", got)
	}
}

// TestSetProjectEnvVars_FullReplace proves the second call removes a key
// the first call set but the second omits, not a merge.
func TestSetProjectEnvVars_FullReplace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, newTestProject()); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatalf("first SetProjectEnvVars() error = %v", err)
	}
	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"B": "3", "C": "4"}); err != nil {
		t.Fatalf("second SetProjectEnvVars() error = %v", err)
	}

	got, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	want := map[string]string{"B": "3", "C": "4"}
	if len(got) != len(want) {
		t.Fatalf("ListProjectEnvVars() = %+v, want %+v (A must be gone)", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListProjectEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestSetProjectEnvVars_ProjectDeletedCascades proves the FK's ON DELETE
// CASCADE actually behaves the way the migration's own comment claims: a
// deleted project's shared env vars don't linger as orphaned rows.
func TestSetProjectEnvVars_ProjectDeletedCascades(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, newTestProject()); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"A": "1"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	if err := db.DeleteProject(ctx, "proj_test1"); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	got, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListProjectEnvVars() after project delete = %+v, want empty (cascade deleted)", got)
	}
}
