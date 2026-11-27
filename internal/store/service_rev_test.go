package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestSaveDesiredServiceRejectsStaleSnapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	snap, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2"}, nil); err != nil {
		t.Fatal(err)
	}
	stale := *snap
	stale.Image = "x:2"
	stale.CheckRev = true
	if err := db.SaveDesiredService(ctx, stale); !errors.Is(err, ErrStaleDesiredService) {
		t.Fatalf("stale save err = %v, want ErrStaleDesiredService", err)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if got.Image != "x:1" || got.Env["B"] != "2" {
		t.Fatalf("row after rejected save = %+v", got)
	}
	// Without CheckRev the legacy full replace still applies.
	stale.CheckRev = false
	if err := db.SaveDesiredService(ctx, stale); err != nil {
		t.Fatal(err)
	}
}

func TestSaveRebasedKeepsEnvWrittenAfterSnapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	snap, _ := db.GetDesiredService(ctx, "web")
	if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2"}, nil); err != nil {
		t.Fatal(err)
	}
	build := func(base DesiredService) DesiredService { base.Image = "x:2"; return base }
	if _, err := SaveRebased(ctx, build(*snap), db.GetDesiredService, db.SaveDesiredService, build); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if got.Image != "x:2" || got.Env["A"] != "1" || got.Env["B"] != "2" {
		t.Fatalf("row = image %q env %v, want x:2 with A and B", got.Image, got.Env)
	}
}

func TestSaveRebasedConcurrentWithEnvMerges(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:0", Port: 80}); err != nil {
		t.Fatal(err)
	}
	const keys = 25
	var wg sync.WaitGroup
	for i := range keys {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{fmt.Sprintf("K%d", i): "v"}, nil); err != nil {
				t.Errorf("merge: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			snap, err := db.GetDesiredService(ctx, "web")
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			build := func(base DesiredService) DesiredService { base.Image = fmt.Sprintf("x:%d", i+1); return base }
			// Contention can exhaust the bounded retries; that must surface as a typed error, never a lost key.
			if _, err := SaveRebased(ctx, build(*snap), db.GetDesiredService, db.SaveDesiredService, build); err != nil && !errors.Is(err, ErrStaleDesiredService) {
				t.Errorf("save: %v", err)
			}
		}()
	}
	wg.Wait()
	got, _ := db.GetDesiredService(ctx, "web")
	for i := range keys {
		if got.Env[fmt.Sprintf("K%d", i)] != "v" {
			t.Fatalf("key K%d lost, env has %d keys", i, len(got.Env))
		}
	}
}

func TestMigration3344AddsRevAndBumpsOnSpecWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3343))
	if err != nil {
		t.Fatal(err)
	}
	// Raw insert: the save SQL already names the rev column this migration adds.
	if _, err := old.ExecContext(ctx, `INSERT INTO desired_services (name, image, port, updated_at) VALUES ('web', 'x:1', 80, '2026-11-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()
	rev := func() int64 { s, _ := cur.GetDesiredService(ctx, "web"); return s.Rev }
	if rev() != 0 {
		t.Fatalf("legacy row rev = %d, want 0", rev())
	}
	if _, err := cur.MergeServiceEnv(ctx, "web", map[string]string{"A": "1"}, nil); err != nil {
		t.Fatal(err)
	}
	if rev() != 1 {
		t.Fatalf("rev after env merge = %d, want 1", rev())
	}
	if err := cur.UpdateServiceNode(ctx, "web", "n1"); err != nil {
		t.Fatal(err)
	}
	if rev() != 1 {
		t.Fatalf("rev bumped by a non-spec column: %d", rev())
	}
}
