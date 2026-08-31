package store

import (
	"context"
	"errors"
	"testing"
)

func seedTestProject(t *testing.T, db *DB) {
	t.Helper()
	if err := db.SaveProject(context.Background(), Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func TestSaveAndGetEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)

	want := Environment{ID: "env_test1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}
	if err := db.SaveEnvironment(ctx, want); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	got, err := db.GetEnvironment(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got != want {
		t.Errorf("GetEnvironment() = %+v, want %+v", got, want)
	}
}

func TestGetEnvironment_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetEnvironment(context.Background(), "env_missing")
	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestListEnvironmentsByProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)
	if err := db.SaveProject(ctx, Project{ID: "proj_2", Name: "other", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed other project: %v", err)
	}

	if err := db.SaveEnvironment(ctx, Environment{ID: "env_a", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveEnvironment(ctx, Environment{ID: "env_b", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:01Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveEnvironment(ctx, Environment{ID: "env_c", ProjectID: "proj_2", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed unrelated project: %v", err)
	}

	got, err := db.ListEnvironmentsByProject(ctx, "proj_1")
	if err != nil {
		t.Fatalf("ListEnvironmentsByProject() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "env_a" || got[1].ID != "env_b" {
		t.Fatalf("ListEnvironmentsByProject() = %+v, want [env_a, env_b], scoped to proj_1", got)
	}
}

func TestDeleteEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)

	env := Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}
	if err := db.SaveEnvironment(ctx, env); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := db.DeleteEnvironment(ctx, env.ID); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}
	if _, err := db.GetEnvironment(ctx, env.ID); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() after delete error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestDeleteEnvironment_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeleteEnvironment(context.Background(), "env_missing")
	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("DeleteEnvironment() error = %v, want ErrEnvironmentNotFound", err)
	}
}

// TestDeleteProject_CascadesEnvironments documents environments.project_id's
// ON DELETE CASCADE (migrations/0055): unlike a project's own relationship
// to organizations/apps, an environment is owned by its project, so
// deleting the project must delete its environments, not orphan them.
func TestDeleteProject_CascadesEnvironments(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)

	env := Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}
	if err := db.SaveEnvironment(ctx, env); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := db.DeleteProject(ctx, "proj_1"); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	if _, err := db.GetEnvironment(ctx, env.ID); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() after owning project deleted error = %v, want ErrEnvironmentNotFound (cascaded)", err)
	}
}

func TestSetEnvironmentProtected_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)

	env := Environment{ID: "env_1", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:00Z"}
	if err := db.SaveEnvironment(ctx, env); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	if err := db.SetEnvironmentProtected(ctx, env.ID, true); err != nil {
		t.Fatalf("SetEnvironmentProtected(true) error = %v", err)
	}
	got, err := db.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if !got.Protected {
		t.Errorf("Protected = false after SetEnvironmentProtected(true), want true")
	}

	if err := db.SetEnvironmentProtected(ctx, env.ID, false); err != nil {
		t.Fatalf("SetEnvironmentProtected(false) error = %v", err)
	}
	got, err = db.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.Protected {
		t.Errorf("Protected = true after SetEnvironmentProtected(false), want false")
	}
}

func TestSetEnvironmentProtected_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.SetEnvironmentProtected(context.Background(), "env_missing", true)
	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("SetEnvironmentProtected() error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestSetServiceEnvironment_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)
	if err := db.SaveEnvironment(ctx, Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed service: %v", err)
	}

	if err := db.SetServiceEnvironment(ctx, "web", "env_1"); err != nil {
		t.Fatalf("SetServiceEnvironment() error = %v", err)
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.EnvironmentID != "env_1" {
		t.Errorf("EnvironmentID = %q, want env_1", got.EnvironmentID)
	}
}

func TestSetServiceEnvironment_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.SetServiceEnvironment(context.Background(), "ghost", "env_1")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("SetServiceEnvironment() error = %v, want ErrServiceNotFound", err)
	}
}

// TestDeleteEnvironment_ServiceBecomesUntagged is the regression
// desired_services.environment_id's ON DELETE SET NULL foreign key
// exists to prevent: deleting an environment must leave tagged services
// running, untagged again.
func TestDeleteEnvironment_ServiceBecomesUntagged(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)
	if err := db.SaveEnvironment(ctx, Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_1"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	if err := db.DeleteEnvironment(ctx, "env_1"); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v, want no error", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() after environment delete error = %v, want the service to still exist", err)
	}
	if got.EnvironmentID != "" {
		t.Errorf("EnvironmentID = %q after owning environment deleted, want empty", got.EnvironmentID)
	}
}
