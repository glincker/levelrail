package store

import (
	"context"
	"errors"
	"testing"
)

func newTestOrganization() Organization {
	return Organization{ID: "org_test1", Name: "acme", CreatedAt: "2026-08-20T00:00:00Z"}
}

func TestSaveAndGetOrganization(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestOrganization()
	if err := db.SaveOrganization(ctx, want); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

	got, err := db.GetOrganization(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetOrganization() error = %v", err)
	}
	if got != want {
		t.Errorf("GetOrganization() = %+v, want %+v", got, want)
	}
}

func TestGetOrganization_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetOrganization(ctx, "org_missing")
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("GetOrganization() error = %v, want ErrOrganizationNotFound", err)
	}
}

func TestListOrganizations_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveOrganization(ctx, Organization{ID: "org_b", Name: "b", CreatedAt: "2026-08-20T00:00:01Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveOrganization(ctx, Organization{ID: "org_a", Name: "a", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := db.ListOrganizations(ctx)
	if err != nil {
		t.Fatalf("ListOrganizations() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "org_a" || got[1].ID != "org_b" {
		t.Fatalf("ListOrganizations() = %+v, want [org_a, org_b] in creation order", got)
	}
}

func TestDeleteOrganization(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestOrganization()
	if err := db.SaveOrganization(ctx, want); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}
	if err := db.DeleteOrganization(ctx, want.ID); err != nil {
		t.Fatalf("DeleteOrganization() error = %v", err)
	}

	_, err := db.GetOrganization(ctx, want.ID)
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("GetOrganization() after delete error = %v, want ErrOrganizationNotFound", err)
	}
}

func TestDeleteOrganization_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteOrganization(ctx, "org_missing")
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("DeleteOrganization() error = %v, want ErrOrganizationNotFound", err)
	}
}

func TestSetProjectOrganization_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if err := db.SetProjectOrganization(ctx, "proj_1", "org_test1"); err != nil {
		t.Fatalf("SetProjectOrganization() error = %v", err)
	}
	got, err := db.GetProject(ctx, "proj_1")
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.OrgID != "org_test1" {
		t.Errorf("OrgID = %q, want org_test1", got.OrgID)
	}
}

func TestSetProjectOrganization_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.SetProjectOrganization(ctx, "proj_missing", "org_1")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("SetProjectOrganization() error = %v, want ErrProjectNotFound", err)
	}
}

// TestDeleteOrganization_ProjectBecomesOrgLess is the regression this
// table's ON DELETE SET NULL foreign key exists to prevent: deleting an
// organization must leave its projects running, org-less again.
func TestDeleteOrganization_ProjectBecomesOrgLess(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveOrganization(ctx, newTestOrganization()); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SetProjectOrganization(ctx, "proj_1", "org_test1"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	if err := db.DeleteOrganization(ctx, "org_test1"); err != nil {
		t.Fatalf("DeleteOrganization() error = %v, want no error", err)
	}

	got, err := db.GetProject(ctx, "proj_1")
	if err != nil {
		t.Fatalf("GetProject() after org delete error = %v, want the project to still exist", err)
	}
	if got.OrgID != "" {
		t.Errorf("OrgID = %q after owning org was deleted, want empty", got.OrgID)
	}
}
