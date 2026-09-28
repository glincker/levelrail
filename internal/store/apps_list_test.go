package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func seedListApps(t *testing.T, db *DB, n int) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, Project{ID: "p1", Name: "proj", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, Environment{ID: "e1", ProjectID: "p1", Name: "staging", CreatedAt: "x"}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, created_at) VALUES ('t1','team:core','x'),('t2','tier:web','x')`); err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("app-%04d", i)
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:1", Port: 80}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		if i%2 == 0 {
			if err := db.SetServiceEnvironment(ctx, name, "e1"); err != nil {
				t.Fatalf("set env: %v", err)
			}
			if err := db.UpdateServiceProject(ctx, name, "p1"); err != nil {
				t.Fatalf("set project: %v", err)
			}
			if err := db.AttachAppTag(ctx, "t1", name); err != nil {
				t.Fatalf("attach: %v", err)
			}
		}
		if i%3 == 0 {
			if err := db.AttachAppTag(ctx, "t2", name); err != nil {
				t.Fatalf("attach: %v", err)
			}
		}
	}
}

func TestListDesiredServicesFiltered(t *testing.T) {
	db := openTestDB(t)
	seedListApps(t, db, 600)
	ctx := context.Background()
	tests := []struct {
		name      string
		f         AppListFilter
		wantTotal int
		wantRows  int
	}{
		{"no filter paged", AppListFilter{Limit: 50}, 600, 50},
		{"environment by name", AppListFilter{Environment: "staging"}, 300, 300},
		{"environment by id paged", AppListFilter{Environment: "e1", Limit: 10, Offset: 295}, 300, 5},
		{"project", AppListFilter{ProjectID: "p1"}, 300, 300},
		{"tag", AppListFilter{Tags: []string{"tier:web"}}, 200, 200},
		{"tags and", AppListFilter{Tags: []string{"tier:web", "team:core"}}, 100, 100},
		{"query", AppListFilter{Query: "app-059"}, 10, 10},
		{"query underscore is literal", AppListFilter{Query: "app_"}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			got, total, err := db.ListDesiredServicesFiltered(ctx, tc.f)
			if err != nil {
				t.Fatal(err)
			}
			if total != tc.wantTotal || len(got) != tc.wantRows {
				t.Errorf("total=%d rows=%d, want %d/%d", total, len(got), tc.wantTotal, tc.wantRows)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("query took %s, over budget", d)
			}
		})
	}
}
