package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEnvRevisions_PruneByCountAndAge(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for range 5 {
		if _, err := db.AddEnvRevision(ctx, "web", "me", []string{"A"}, []byte("x"), 3, 0); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListEnvRevisions(ctx, "web", 50)
	if err != nil || len(got) != 3 || got[0].Rev != 5 || got[2].Rev != 3 {
		t.Fatalf("after count prune = %+v, %v", got, err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour).Format(envRevisionTimeLayout)
	if _, err := db.ExecContext(ctx, `UPDATE env_revisions SET at = ? WHERE rev < 5`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddEnvRevision(ctx, "web", "me", nil, nil, 10, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListEnvRevisions(ctx, "web", 50)
	if len(got) != 2 || got[1].Rev != 5 {
		t.Fatalf("after age prune = %+v", got)
	}
	if _, err := db.GetEnvRevision(ctx, "web", 1); !errors.Is(err, ErrEnvRevisionNotFound) {
		t.Fatalf("pruned revision err = %v", err)
	}
}

func TestDeleteDesiredService_ClearsEnvRevisions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddEnvRevision(ctx, "web", "me", []string{"A"}, []byte("x"), 5, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if got, err := db.ListEnvRevisions(ctx, "web", 50); err != nil || len(got) != 0 {
		t.Fatalf("recreated app revisions = %+v, %v", got, err)
	}
}
