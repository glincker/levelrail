package store

import (
	"context"
	"errors"
	"testing"
)

func TestGlobalProject_IsHiddenAndUndeletable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	projects, err := db.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	for _, p := range projects {
		if p.ID == GlobalProjectID {
			t.Errorf("ListProjects() returned the reserved project %q", p.ID)
		}
	}
	if err := db.DeleteProject(ctx, GlobalProjectID); !errors.Is(err, ErrReservedProject) {
		t.Errorf("DeleteProject(global) error = %v, want ErrReservedProject", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM environments WHERE scope = 'global'`).Scan(&n); err != nil || n != 4 {
		t.Errorf("global environments after a refused delete = %d (err %v), want the 4 seeded", n, err)
	}
}
