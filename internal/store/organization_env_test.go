package store

import (
	"context"
	"testing"
)

func TestSetAndListOrganizationEnvVars(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

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
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

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
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

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
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}
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
