package store

import (
	"context"
	"errors"
	"testing"
)

func newTestProject() Project {
	return Project{
		ID:        "proj_test1",
		Name:      "my-saas",
		CreatedAt: "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestProject()
	if err := db.SaveProject(ctx, want); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	got, err := db.GetProject(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got != want {
		t.Errorf("GetProject() = %+v, want %+v", got, want)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetProject(ctx, "proj_missing")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("GetProject() error = %v, want ErrProjectNotFound", err)
	}
}

func TestListProjects_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestProject()
	first.ID, first.CreatedAt = "proj_a", "2026-08-14T00:00:00Z"
	second := newTestProject()
	second.ID, second.CreatedAt = "proj_b", "2026-08-14T00:00:01Z"

	if err := db.SaveProject(ctx, second); err != nil {
		t.Fatalf("SaveProject(second) error = %v", err)
	}
	if err := db.SaveProject(ctx, first); err != nil {
		t.Fatalf("SaveProject(first) error = %v", err)
	}

	got, err := db.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "proj_a" || got[1].ID != "proj_b" {
		t.Fatalf("ListProjects() = %+v, want [proj_a, proj_b] in creation order", got)
	}
}

// TestListProjects_DuplicateNamesAllowed documents the deliberate
// absence of a UNIQUE constraint on projects.name (migrations/
// 0022_projects.sql's own comment on why): a project is addressed by
// id, its name is a pure label, so two projects sharing a display name
// is valid, not a conflict.
func TestListProjects_DuplicateNamesAllowed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	a := Project{ID: "proj_a", Name: "shared-name", CreatedAt: "2026-08-14T00:00:00Z"}
	b := Project{ID: "proj_b", Name: "shared-name", CreatedAt: "2026-08-14T00:00:01Z"}
	if err := db.SaveProject(ctx, a); err != nil {
		t.Fatalf("SaveProject(a) error = %v", err)
	}
	if err := db.SaveProject(ctx, b); err != nil {
		t.Fatalf("SaveProject(b) error = %v, want no error (duplicate names are allowed)", err)
	}

	got, err := db.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListProjects() len = %d, want 2", len(got))
	}
}

func TestDeleteProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestProject()
	if err := db.SaveProject(ctx, want); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.DeleteProject(ctx, want.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	_, err := db.GetProject(ctx, want.ID)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("GetProject() after delete error = %v, want ErrProjectNotFound", err)
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteProject(ctx, "proj_missing")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("DeleteProject() error = %v, want ErrProjectNotFound", err)
	}
}

// TestDeleteProject_ServiceBecomesProjectLess is the real regression
// this table's ON DELETE SET NULL foreign key (migrations/
// 0022_projects.sql) exists to prevent: unlike backup_targets (whose
// foreign key rejects a delete while referenced), deleting a project
// must never delete or block deletion of the apps that belonged to it,
// it must leave them running with project_id reverted to "no project".
func TestDeleteProject_ServiceBecomesProjectLess(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	proj := newTestProject()
	if err := db.SaveProject(ctx, proj); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", proj.ID); err != nil {
		t.Fatalf("UpdateServiceProject() error = %v", err)
	}

	if err := db.DeleteProject(ctx, proj.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v, want no error (deleting a project must not be blocked by, or cascade to, its resources)", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() after project delete error = %v, want the service to still exist", err)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q after owning project was deleted, want empty (project-less, not deleted)", got.ProjectID)
	}
}

// TestDeleteProject_DatabaseBecomesProjectLess is
// TestDeleteProject_ServiceBecomesProjectLess's database-kind
// counterpart.
func TestDeleteProject_DatabaseBecomesProjectLess(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	proj := newTestProject()
	if err := db.SaveProject(ctx, proj); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "mydb", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "mydb", proj.ID); err != nil {
		t.Fatalf("UpdateDatabaseProject() error = %v", err)
	}

	if err := db.DeleteProject(ctx, proj.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v, want no error", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "mydb")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() after project delete error = %v, want the database to still exist", err)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q after owning project was deleted, want empty (project-less, not deleted)", got.ProjectID)
	}
}
