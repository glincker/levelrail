package store

import (
	"context"
	"testing"
)

// newSeededOrganizationDB opens a test DB with "org_test1" already
// saved, the fixed preamble most tests in this file need before they
// can touch organization-scoped env vars.
func newSeededOrganizationDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}
	return db, ctx
}

func TestSetAndListOrganizationEnvVars(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)

	want := map[string]string{"LOG_LEVEL": "info", "NODE_ENV": "production"}
	if err := db.SetOrganizationEnvVars(ctx, "org_test1", want); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}

	got, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ListOrganizationEnvVars() = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListOrganizationEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestListOrganizationEnvVars_NoneSet_ReturnsEmptyNotNil(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)

	got, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if got == nil {
		t.Error("ListOrganizationEnvVars() = nil, want an empty map")
	}
	if len(got) != 0 {
		t.Errorf("ListOrganizationEnvVars() = %+v, want empty", got)
	}
}

// TestSetOrganizationEnvVars_FullReplace proves the second call removes a
// key the first call set but the second omits, not a merge.
func TestSetOrganizationEnvVars_FullReplace(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)

	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatalf("first SetOrganizationEnvVars() error = %v", err)
	}
	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"B": "3", "C": "4"}); err != nil {
		t.Fatalf("second SetOrganizationEnvVars() error = %v", err)
	}

	got, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	want := map[string]string{"B": "3", "C": "4"}
	if len(got) != len(want) {
		t.Fatalf("ListOrganizationEnvVars() = %+v, want %+v (A must be gone)", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ListOrganizationEnvVars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestSetOrganizationEnvVars_OrganizationDeletedCascades proves the FK's
// ON DELETE CASCADE actually behaves the way the migration's own comment
// claims: a deleted organization's shared env vars don't linger as
// orphaned rows.
func TestSetOrganizationEnvVars_OrganizationDeletedCascades(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)
	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"A": "1"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}

	if err := db.DeleteOrganization(ctx, "org_test1"); err != nil {
		t.Fatalf("DeleteOrganization() error = %v", err)
	}

	got, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListOrganizationEnvVars() after organization delete = %+v, want empty (cascade deleted)", got)
	}
}

func TestListOrganizationEnvVarsForProject(t *testing.T) {
	tests := []struct {
		name      string
		seedOrg   bool
		assignOrg bool
		orgVars   map[string]string
		projectID string
		want      map[string]string
	}{
		{
			name:      "project has an org with vars set",
			seedOrg:   true,
			assignOrg: true,
			orgVars:   map[string]string{"LOG_LEVEL": "info"},
			projectID: "proj_test1",
			want:      map[string]string{"LOG_LEVEL": "info"},
		},
		{
			name:      "project has no org assigned",
			seedOrg:   true,
			assignOrg: false,
			orgVars:   map[string]string{"LOG_LEVEL": "info"},
			projectID: "proj_test1",
			want:      map[string]string{},
		},
		{
			name:      "project does not exist",
			projectID: "proj_missing",
			want:      map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			if err := db.SaveProject(ctx, newTestProject()); err != nil {
				t.Fatalf("SaveProject() error = %v", err)
			}
			if tt.seedOrg {
				if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
					t.Fatalf("SaveOrganization() error = %v", err)
				}
				if err := db.SetOrganizationEnvVars(ctx, "org_test1", tt.orgVars); err != nil {
					t.Fatalf("SetOrganizationEnvVars() error = %v", err)
				}
			}
			if tt.assignOrg {
				if err := db.SetProjectOrganization(ctx, "proj_test1", "org_test1"); err != nil {
					t.Fatalf("SetProjectOrganization() error = %v", err)
				}
			}

			got, err := db.ListOrganizationEnvVarsForProject(ctx, tt.projectID)
			if err != nil {
				t.Fatalf("ListOrganizationEnvVarsForProject() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ListOrganizationEnvVarsForProject() = %+v, want %+v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("ListOrganizationEnvVarsForProject()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestGetProjectOrganizationID(t *testing.T) {
	tests := []struct {
		name        string
		saveProject bool
		assignOrg   bool
		projectID   string
		want        string
	}{
		{name: "project has an org", saveProject: true, assignOrg: true, projectID: "proj_test1", want: "org_test1"},
		{name: "project has no org", saveProject: true, assignOrg: false, projectID: "proj_test1", want: ""},
		{name: "project does not exist", saveProject: false, projectID: "proj_missing", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			if tt.saveProject {
				if err := db.SaveProject(ctx, newTestProject()); err != nil {
					t.Fatalf("SaveProject() error = %v", err)
				}
			}
			if tt.assignOrg {
				if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
					t.Fatalf("SaveOrganization() error = %v", err)
				}
				if err := db.SetProjectOrganization(ctx, "proj_test1", "org_test1"); err != nil {
					t.Fatalf("SetProjectOrganization() error = %v", err)
				}
			}

			got, err := db.GetProjectOrganizationID(ctx, tt.projectID)
			if err != nil {
				t.Fatalf("GetProjectOrganizationID() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("GetProjectOrganizationID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrganizationEnvSecretsKey(t *testing.T) {
	got := OrganizationEnvSecretsKey("org_test1")
	want := "organization-env/org_test1"
	if got != want {
		t.Errorf("OrganizationEnvSecretsKey() = %q, want %q", got, want)
	}
}

// TestSetOrganizationSecretEnvVar covers the upsert path: the first call
// inserts a placeholder row, the second re-marks the same key, and
// neither ever surfaces through the plain env var list.
func TestSetOrganizationSecretEnvVar(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)

	if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
		t.Fatalf("SetOrganizationSecretEnvVar() error = %v", err)
	}
	// upsert again to exercise the ON CONFLICT branch
	if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
		t.Fatalf("SetOrganizationSecretEnvVar() second call error = %v", err)
	}

	keys, err := db.ListOrganizationSecretEnvKeys(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "API_KEY" {
		t.Fatalf("ListOrganizationSecretEnvKeys() = %v, want [API_KEY]", keys)
	}

	plain, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if _, ok := plain["API_KEY"]; ok {
		t.Errorf("ListOrganizationEnvVars() leaked secret key API_KEY = %+v", plain)
	}
}

func TestDeleteOrganizationSecretEnvVar(t *testing.T) {
	tests := []struct {
		name      string
		seedFirst bool
	}{
		{name: "deletes an existing secret", seedFirst: true},
		{name: "no-op when key was never set", seedFirst: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx := newSeededOrganizationDB(t)
			if tt.seedFirst {
				if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
					t.Fatalf("SetOrganizationSecretEnvVar() error = %v", err)
				}
			}

			if err := db.DeleteOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
				t.Fatalf("DeleteOrganizationSecretEnvVar() error = %v", err)
			}

			keys, err := db.ListOrganizationSecretEnvKeys(ctx, "org_test1")
			if err != nil {
				t.Fatalf("ListOrganizationSecretEnvKeys() error = %v", err)
			}
			if len(keys) != 0 {
				t.Errorf("ListOrganizationSecretEnvKeys() = %v, want empty", keys)
			}
		})
	}
}

// TestListOrganizationEnvVarsDetailed proves the combined shape used by
// the settings-page table: plain rows carry their value, secret rows
// never do, and results come back key-ordered.
func TestListOrganizationEnvVarsDetailed(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)
	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"NODE_ENV": "production"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}
	if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
		t.Fatalf("SetOrganizationSecretEnvVar() error = %v", err)
	}

	got, err := db.ListOrganizationEnvVarsDetailed(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVarsDetailed() error = %v", err)
	}
	want := []SharedEnvVar{
		{Key: "API_KEY", Value: "", Secret: true},
		{Key: "NODE_ENV", Value: "production", Secret: false},
	}
	if len(got) != len(want) {
		t.Fatalf("ListOrganizationEnvVarsDetailed() = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i].Key != w.Key || got[i].Value != w.Value || got[i].Secret != w.Secret {
			t.Errorf("ListOrganizationEnvVarsDetailed()[%d] = %+v, want Key/Value/Secret %+v", i, got[i], w)
		}
	}
}

// TestSetOrganizationEnvVars_LeavesUnrelatedSecretRowIntact proves the
// full-replace PUT (SetOrganizationEnvVars) never touches a
// secret-marked row for a key it wasn't given.
func TestSetOrganizationEnvVars_LeavesUnrelatedSecretRowIntact(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)
	if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
		t.Fatalf("SetOrganizationSecretEnvVar() error = %v", err)
	}

	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"NODE_ENV": "production"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}

	keys, err := db.ListOrganizationSecretEnvKeys(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "API_KEY" {
		t.Fatalf("ListOrganizationSecretEnvKeys() = %v, want [API_KEY] to survive", keys)
	}

	plain, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if _, ok := plain["API_KEY"]; ok {
		t.Errorf("ListOrganizationEnvVars() leaked secret key API_KEY = %+v", plain)
	}
	if plain["NODE_ENV"] != "production" {
		t.Errorf("ListOrganizationEnvVars()[NODE_ENV] = %q, want production", plain["NODE_ENV"])
	}
}

// TestSetOrganizationEnvVars_SameKeyAsSecret_ConvertsToPlain documents
// the intentional "last write wins" behavior: reusing a secret-marked
// key through the plain full-replace path converts that row back to
// plain rather than erroring or duplicating it.
func TestSetOrganizationEnvVars_SameKeyAsSecret_ConvertsToPlain(t *testing.T) {
	db, ctx := newSeededOrganizationDB(t)
	if err := db.SetOrganizationSecretEnvVar(ctx, "org_test1", "API_KEY"); err != nil {
		t.Fatalf("SetOrganizationSecretEnvVar() error = %v", err)
	}

	if err := db.SetOrganizationEnvVars(ctx, "org_test1", map[string]string{"API_KEY": "plain-value"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}

	keys, err := db.ListOrganizationSecretEnvKeys(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("ListOrganizationSecretEnvKeys() = %v, want empty (converted to plain)", keys)
	}

	plain, err := db.ListOrganizationEnvVars(ctx, "org_test1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if plain["API_KEY"] != "plain-value" {
		t.Errorf("ListOrganizationEnvVars()[API_KEY] = %q, want plain-value", plain["API_KEY"])
	}
}
