package store

import (
	"context"
	"testing"
)

func seedTestEnvironment(t *testing.T, db *DB) {
	t.Helper()
	seedTestProject(t, db)
	if err := db.SaveEnvironment(context.Background(), Environment{
		ID: "env_test1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-25T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
}

func TestSetAndListEnvironmentEnvVars(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestEnvironment(t, db)

	want := map[string]string{"LOG_LEVEL": "info", "NODE_ENV": "staging"}
	if err := db.SetEnvironmentEnvVars(ctx, "env_test1", want); err != nil {
		t.Fatalf("SetEnvironmentEnvVars() error = %v", err)
	}

	got, err := db.ListEnvironmentEnvVars(ctx, "env_test1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ListEnvironmentEnvVars() = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListEnvironmentEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestListEnvironmentEnvVars_NoneSet_ReturnsEmptyNotNil(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestEnvironment(t, db)

	got, err := db.ListEnvironmentEnvVars(ctx, "env_test1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	if got == nil {
		t.Error("ListEnvironmentEnvVars() = nil, want an empty map")
	}
	if len(got) != 0 {
		t.Errorf("ListEnvironmentEnvVars() = %+v, want empty", got)
	}
}

// TestSetEnvironmentEnvVars_FullReplace proves the second call removes a
// key the first call set but the second omits, not a merge.
func TestSetEnvironmentEnvVars_FullReplace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestEnvironment(t, db)

	if err := db.SetEnvironmentEnvVars(ctx, "env_test1", map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatalf("first SetEnvironmentEnvVars() error = %v", err)
	}
	if err := db.SetEnvironmentEnvVars(ctx, "env_test1", map[string]string{"B": "3", "C": "4"}); err != nil {
		t.Fatalf("second SetEnvironmentEnvVars() error = %v", err)
	}

	got, err := db.ListEnvironmentEnvVars(ctx, "env_test1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	want := map[string]string{"B": "3", "C": "4"}
	if len(got) != len(want) {
		t.Fatalf("ListEnvironmentEnvVars() = %+v, want %+v (A must be gone)", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListEnvironmentEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestSetEnvironmentEnvVars_EnvironmentDeletedCascades proves the FK's ON
// DELETE CASCADE actually behaves the way the migration's own comment
// claims: a deleted environment's shared env vars don't linger as
// orphaned rows.
func TestSetEnvironmentEnvVars_EnvironmentDeletedCascades(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestEnvironment(t, db)
	if err := db.SetEnvironmentEnvVars(ctx, "env_test1", map[string]string{"A": "1"}); err != nil {
		t.Fatalf("SetEnvironmentEnvVars() error = %v", err)
	}

	if err := db.DeleteEnvironment(ctx, "env_test1"); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}

	got, err := db.ListEnvironmentEnvVars(ctx, "env_test1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListEnvironmentEnvVars() after environment delete = %+v, want empty (cascade deleted)", got)
	}
}
