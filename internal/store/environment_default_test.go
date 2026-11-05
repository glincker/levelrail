package store

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultEnvironment_SetClearsPreviousAndScopesToProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)
	if err := db.SaveProject(ctx, Project{ID: "proj_2", Name: "other", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []Environment{
		{ID: "env_a", ProjectID: "proj_1", Name: "a", CreatedAt: "2026-08-20T00:00:00Z"},
		{ID: "env_b", ProjectID: "proj_1", Name: "b", CreatedAt: "2026-08-20T00:00:00Z"},
		{ID: "env_x", ProjectID: "proj_2", Name: "x", CreatedAt: "2026-08-20T00:00:00Z"},
	} {
		if err := db.SaveEnvironment(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := db.DefaultEnvironmentID(ctx, "proj_1"); got != "" {
		t.Fatalf("default before set = %q, want none", got)
	}
	if err := db.SetProjectDefaultEnvironment(ctx, "proj_1", "env_a"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetProjectDefaultEnvironment(ctx, "proj_1", "env_b"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.DefaultEnvironmentID(ctx, "proj_1"); got != "env_b" {
		t.Errorf("default = %q, want env_b", got)
	}
	if err := db.SetProjectDefaultEnvironment(ctx, "proj_1", "env_x"); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Errorf("other project's environment error = %v, want ErrEnvironmentNotFound", err)
	}
	if err := db.DeleteEnvironment(ctx, "env_b"); !errors.Is(err, ErrDefaultEnvironment) {
		t.Errorf("delete default error = %v, want ErrDefaultEnvironment", err)
	}
	missing, err := db.ProjectsWithoutDefaultEnvironment(ctx)
	if err != nil || len(missing) != 1 || missing[0].ID != "proj_2" {
		t.Errorf("projects without default = %v, %v; want only proj_2", missing, err)
	}
}

func TestSaveDefaultEnvironment_ReplacesPreviousDefault(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTestProject(t, db)
	for _, id := range []string{"env_1", "env_2"} {
		if err := db.SaveDefaultEnvironment(ctx, Environment{ID: id, ProjectID: "proj_1", Name: id, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := db.DefaultEnvironmentID(ctx, "proj_1"); got != "env_2" {
		t.Errorf("default = %q, want env_2", got)
	}
}
