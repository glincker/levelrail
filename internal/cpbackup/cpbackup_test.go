package cpbackup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestManager_CreateListPrune(t *testing.T) {
	ctx := context.Background()
	m := NewManager(openDB(t), t.TempDir())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := 0
	m.now = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Minute) }

	manual, err := m.Create(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := m.CreateScheduled(ctx); err != nil {
			t.Fatalf("scheduled: %v", err)
		}
	}
	n, err := m.Prune(2)
	if err != nil || n != 2 {
		t.Fatalf("prune = %d, %v; want 2", n, err)
	}
	list, err := m.List()
	if err != nil || len(list) != 3 {
		t.Fatalf("list = %d, %v; want 3 (1 manual + 2 scheduled)", len(list), err)
	}
	if list[0].Name < list[1].Name {
		t.Errorf("list not newest first: %v", list)
	}
	found := false
	for _, i := range list {
		found = found || i.Name == manual.Name
	}
	if !found {
		t.Error("manual backup was pruned")
	}
}

func TestManager_SameSecondDoesNotCollide(t *testing.T) {
	ctx := context.Background()
	m := NewManager(openDB(t), t.TempDir())
	m.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	a, err := m.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name == b.Name {
		t.Fatal("names collided")
	}
}
