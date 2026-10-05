package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newFinalizerStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func pendingNames(t *testing.T, db *store.DB) []string {
	t.Helper()
	list, err := db.ListPendingTeardowns(context.Background())
	if err != nil {
		t.Fatalf("ListPendingTeardowns() error = %v", err)
	}
	names := make([]string, 0, len(list))
	for _, p := range list {
		names = append(names, p.Name)
	}
	return names
}

func TestDeleteFinalizer_RemoveFailsOnceThenSucceeds(t *testing.T) {
	ctx := context.Background()
	db := newFinalizerStore(t)
	rt := newFakeRuntime(0)
	target := ContainerName("web", "img:v1", "")
	rt.seed(target, true)
	rt.removeErr = errors.New("busy")
	f := NewDeleteFinalizer(db, func(string) (docker.Runtime, error) { return rt, nil }, nil)

	if err := f.Begin(ctx, "web", ""); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := f.Finalize(ctx, "web", ""); err == nil {
		t.Fatal("Finalize() error = nil, want the remove failure surfaced")
	}
	list, _ := db.ListPendingTeardowns(ctx)
	if len(list) != 1 || list[0].Attempts != 1 || list[0].LastError == "" {
		t.Fatalf("tombstone after failure = %+v, want 1 entry with attempts=1 and an error", list)
	}

	rt.removeErr = nil
	if _, err := f.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if names := rt.names(); len(names) != 0 {
		t.Errorf("containers after retry = %v, want none", names)
	}
	if got := pendingNames(t, db); len(got) != 0 {
		t.Errorf("tombstones after retry = %v, want none", got)
	}
}

func TestDeleteFinalizer_NodeUnreachableThenReturns(t *testing.T) {
	ctx := context.Background()
	db := newFinalizerStore(t)
	rt := newFakeRuntime(0)
	rt.seed(ContainerName("web", "img:v1", ""), true)
	online := false
	f := NewDeleteFinalizer(db, func(nodeID string) (docker.Runtime, error) {
		if nodeID != "node_b" || !online {
			return nil, errors.New("node not registered")
		}
		return rt, nil
	}, nil)

	if err := f.Begin(ctx, "web", "node_b"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := f.Reconcile(ctx); err == nil {
		t.Fatal("Reconcile() error = nil, want an error while the node is unreachable")
	}
	if got := pendingNames(t, db); len(got) != 1 {
		t.Fatalf("tombstones while offline = %v, want web retained", got)
	}
	if len(rt.names()) != 1 {
		t.Fatal("container must stay until the node returns")
	}

	online = true
	if _, err := f.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() after node returned error = %v", err)
	}
	if len(rt.names()) != 0 || len(pendingNames(t, db)) != 0 {
		t.Errorf("after node returned: containers=%v tombstones=%v, want none", rt.names(), pendingNames(t, db))
	}
}

func TestDeleteFinalizer_RecreatedAppIsNotTornDown(t *testing.T) {
	ctx := context.Background()
	db := newFinalizerStore(t)
	rt := newFakeRuntime(0)
	target := ContainerName("web", "img:v2", "")
	rt.seed(target, true)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v2", Port: 3000}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	f := NewDeleteFinalizer(db, func(string) (docker.Runtime, error) { return rt, nil }, nil)
	if err := f.Begin(ctx, "web", ""); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	if _, err := f.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if names := rt.names(); len(names) != 1 || names[0] != target {
		t.Errorf("containers = %v, want the recreated app's container untouched", names)
	}
	if got := pendingNames(t, db); len(got) != 0 {
		t.Errorf("tombstones = %v, want the stale one dropped", got)
	}
}

func TestDeleteFinalizer_OnlyRemovesOwnAppContainers(t *testing.T) {
	ctx := context.Background()
	db := newFinalizerStore(t)
	rt := newFakeRuntime(0)
	mine := ContainerName("web", "img:v1", "")
	sibling := ContainerName("web-worker", "img:v1", "")
	other := ContainerName("webby", "img:v1", "")
	foreign := ContainerName("web", "img:v9", "")
	rt.seed(mine, true)
	rt.seed(sibling, true)
	rt.seed(other, true)
	rt.seedLabeled(foreign, true, map[string]string{"platform-reserved.instance": "inst-b"})
	f := NewDeleteFinalizer(db, func(string) (docker.Runtime, error) { return rt, nil }, nil, WithInstanceID("inst-a"))

	if err := f.Begin(ctx, "web", ""); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := f.Finalize(ctx, "web", ""); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	got := map[string]bool{}
	for _, n := range rt.names() {
		got[n] = true
	}
	if got[mine] || !got[sibling] || !got[other] || !got[foreign] {
		t.Errorf("containers after finalize = %v, want only %q removed", rt.names(), mine)
	}
}
