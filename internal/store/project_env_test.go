package store

import (
	"context"
	"testing"
)

// newSeededProjectDB opens a test DB with "proj_test1" already saved,
// the fixed preamble every test in this file needs before it can touch
// project-scoped env vars.
func newSeededProjectDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, newTestProject()); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	return db, ctx
}

func TestSetAndListProjectEnvVars(t *testing.T) {
	db, ctx := newSeededProjectDB(t)

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
	db, ctx := newSeededProjectDB(t)

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
	db, ctx := newSeededProjectDB(t)

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
	db, ctx := newSeededProjectDB(t)
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

func TestProjectEnvSecretsKey(t *testing.T) {
	got := ProjectEnvSecretsKey("proj_test1")
	want := "project-env/proj_test1"
	if got != want {
		t.Errorf("ProjectEnvSecretsKey() = %q, want %q", got, want)
	}
}

// TestSetProjectSecretEnvVar covers the upsert path: the first call
// inserts a placeholder row, the second re-marks the same key, and
// neither ever surfaces through the plain env var list.
func TestSetProjectSecretEnvVar(t *testing.T) {
	db, ctx := newSeededProjectDB(t)

	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}
	// upsert again to exercise the ON CONFLICT branch
	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() second call error = %v", err)
	}

	keys, err := db.ListProjectSecretEnvKeys(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "API_KEY" {
		t.Fatalf("ListProjectSecretEnvKeys() = %v, want [API_KEY]", keys)
	}

	plain, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if _, ok := plain["API_KEY"]; ok {
		t.Errorf("ListProjectEnvVars() leaked secret key API_KEY = %+v", plain)
	}
}

func TestDeleteProjectSecretEnvVar(t *testing.T) {
	tests := []struct {
		name      string
		seedFirst bool
	}{
		{name: "deletes an existing secret", seedFirst: true},
		{name: "no-op when key was never set", seedFirst: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx := newSeededProjectDB(t)
			if tt.seedFirst {
				if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
					t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
				}
			}

			if err := db.DeleteProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
				t.Fatalf("DeleteProjectSecretEnvVar() error = %v", err)
			}

			keys, err := db.ListProjectSecretEnvKeys(ctx, "proj_test1")
			if err != nil {
				t.Fatalf("ListProjectSecretEnvKeys() error = %v", err)
			}
			if len(keys) != 0 {
				t.Errorf("ListProjectSecretEnvKeys() = %v, want empty", keys)
			}
		})
	}
}

// TestListProjectEnvVarsDetailed proves the combined shape used by the
// settings-page table: plain rows carry their value, secret rows never
// do, and results come back key-ordered.
func TestListProjectEnvVarsDetailed(t *testing.T) {
	db, ctx := newSeededProjectDB(t)
	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"NODE_ENV": "production"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}
	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}

	got, err := db.ListProjectEnvVarsDetailed(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVarsDetailed() error = %v", err)
	}
	want := []SharedEnvVar{
		{Key: "API_KEY", Value: "", Secret: true},
		{Key: "NODE_ENV", Value: "production", Secret: false},
	}
	if len(got) != len(want) {
		t.Fatalf("ListProjectEnvVarsDetailed() = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i].Key != w.Key || got[i].Value != w.Value || got[i].Secret != w.Secret {
			t.Errorf("ListProjectEnvVarsDetailed()[%d] = %+v, want Key/Value/Secret %+v", i, got[i], w)
		}
		if got[i].UpdatedAt.IsZero() {
			t.Errorf("ListProjectEnvVarsDetailed()[%d].UpdatedAt is zero, want a real timestamp", i)
		}
	}
}

// TestSetProjectEnvVars_LeavesUnrelatedSecretRowIntact proves the
// full-replace PUT (SetProjectEnvVars) never touches a secret-marked row
// for a key it wasn't given: its DELETE is scoped to is_secret = 0, so an
// existing secret survives a plain env var update that doesn't mention it.
func TestSetProjectEnvVars_LeavesUnrelatedSecretRowIntact(t *testing.T) {
	db, ctx := newSeededProjectDB(t)
	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}

	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"NODE_ENV": "production"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	keys, err := db.ListProjectSecretEnvKeys(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "API_KEY" {
		t.Fatalf("ListProjectSecretEnvKeys() = %v, want [API_KEY] to survive", keys)
	}

	plain, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if _, ok := plain["API_KEY"]; ok {
		t.Errorf("ListProjectEnvVars() leaked secret key API_KEY = %+v", plain)
	}
	if plain["NODE_ENV"] != "production" {
		t.Errorf("ListProjectEnvVars()[NODE_ENV] = %q, want production", plain["NODE_ENV"])
	}
}

// TestSetProjectEnvVars_SameKeyAsSecret_ConvertsToPlain documents the
// intentional "last write wins" behavior called out in SetProjectEnvVars'
// own comment: reusing a secret-marked key through the plain full-replace
// path converts that row back to plain rather than erroring or duplicating it.
func TestSetProjectEnvVars_SameKeyAsSecret_ConvertsToPlain(t *testing.T) {
	db, ctx := newSeededProjectDB(t)
	if err := db.SetProjectSecretEnvVar(ctx, "proj_test1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}

	if err := db.SetProjectEnvVars(ctx, "proj_test1", map[string]string{"API_KEY": "plain-value"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	keys, err := db.ListProjectSecretEnvKeys(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("ListProjectSecretEnvKeys() = %v, want empty (converted to plain)", keys)
	}

	plain, err := db.ListProjectEnvVars(ctx, "proj_test1")
	if err != nil {
		t.Fatalf("ListProjectEnvVars() error = %v", err)
	}
	if plain["API_KEY"] != "plain-value" {
		t.Errorf("ListProjectEnvVars()[API_KEY] = %q, want plain-value", plain["API_KEY"])
	}
}
