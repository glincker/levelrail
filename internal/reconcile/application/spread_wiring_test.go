package application

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

type lostNodeSub struct{}

func (lostNodeSub) Reconcile(context.Context) (reconcile.Result, error) {
	return notReady(ReasonNodeOffline, errBoom), nil
}
func (lostNodeSub) Teardown(context.Context) error { return errBoom }

func TestSpreadNodeLossReplansAndRetriesCleanup(t *testing.T) {
	ctx := context.Background()
	h := newSpreadHarness(spreadSvc(2), []string{"", "n1"})
	var mu sync.Mutex
	lost := false
	pending := map[string]bool{}
	c := NewSpreadController("web", SpreadDeps{
		Store: &fakeStore{svc: h.svc},
		Plan:  func(context.Context, *store.DesiredService) ([]string, string) { return h.planNode, "" },
		Commit: func(_ context.Context, _ string, nodes []string) error {
			h.svc.ReplicaNodes = nodes
			return nil
		},
		Pending: func() []string {
			mu.Lock()
			defer mu.Unlock()
			var out []string
			for n := range pending {
				out = append(out, n)
			}
			return out
		},
		Build: func(node string, indexes []int, rec DeployRecorder) SpreadSub {
			mu.Lock()
			defer mu.Unlock()
			if node == "n1" && lost {
				pending[node] = true
				return lostNodeSub{}
			}
			if node == "n1" {
				delete(pending, node)
			}
			rt, ok := h.rts[node]
			if !ok {
				rt = newFakeRuntime(0)
				h.rts[node] = rt
			}
			opts := []Option{WithDeployRecorder(rec)}
			if indexes != nil {
				opts = append(opts, WithReplicaIndexes(indexes))
			}
			return New("web", &fakeStore{svc: h.svc}, rt, opts...)
		},
	})

	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.svc.ReplicaNodes, []string{"", "n1"}) || h.rts["n1"].count() != 1 {
		t.Fatalf("after the first pass ReplicaNodes=%v n1=%d, want both nodes serving", h.svc.ReplicaNodes, h.rts["n1"].count())
	}

	lost, h.planNode = true, []string{"", ""}
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if h.svc.ReplicaNodes != nil || h.rts[""].count() != 2 {
		t.Fatalf("after the loss ReplicaNodes=%v home=%d, want a re-plan onto the home node", h.svc.ReplicaNodes, h.rts[""].count())
	}
	if !pending["n1"] || h.rts["n1"].count() != 1 {
		t.Fatalf("pending=%v n1=%d, want n1 queued for cleanup and untouched", pending, h.rts["n1"].count())
	}

	mu.Lock()
	lost = false
	mu.Unlock()
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if h.rts["n1"].count() != 0 || pending["n1"] {
		t.Fatalf("n1=%d pending=%v, want the returning node cleaned", h.rts["n1"].count(), pending)
	}
}

func TestSpreadDeleteRemovesEveryNode(t *testing.T) {
	ctx := context.Background()
	db := newFinalizerStore(t)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Replicas: 2}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceReplicaNodes(ctx, "web", []string{"", "n1"}); err != nil {
		t.Fatal(err)
	}
	rts := map[string]*fakeRuntime{"": newFakeRuntime(0), "n1": newFakeRuntime(0)}
	rts[""].seed(replicaContainerName("web", "img:v1", "", 0), true)
	rts["n1"].seed(replicaContainerName("web", "img:v1", "", 1), true)
	f := NewDeleteFinalizer(db, func(node string) (docker.Runtime, error) { return rts[node], nil }, nil)

	if err := f.Begin(ctx, "web", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if err := f.Finalize(ctx, "web", ""); err != nil {
		t.Fatal(err)
	}
	for node, rt := range rts {
		if rt.count() != 0 {
			t.Errorf("node %q still has %v", node, rt.names())
		}
	}
	if got := pendingNames(t, db); len(got) != 0 {
		t.Errorf("tombstones left: %v", got)
	}
}
