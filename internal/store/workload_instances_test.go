package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func seedWorkloadServices(t *testing.T, db *DB, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := db.SaveDesiredService(context.Background(), DesiredService{Name: n, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkloadInstancesMembership(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedWorkloadServices(t, db, "db", "db-0", "db-1", "web")

	if m, err := db.GetServiceSetMembership(ctx, "web"); err != nil || m.IsChild() {
		t.Fatalf("ordinary service: %+v %v", m, err)
	}
	for i, child := range []string{"db-0", "db-1"} {
		if err := db.SetServiceSetMembership(ctx, child, ServiceSetMembership{ParentService: "db", InstanceKey: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	kids, err := db.ListChildServiceNames(ctx, "db")
	if err != nil || len(kids) != 2 || kids[0] != "db-0" {
		t.Fatalf("children: %v %v", kids, err)
	}
	if err := db.SetServiceSetMembership(ctx, "db", ServiceSetMembership{ParentService: "db", InstanceKey: "x"}); err == nil {
		t.Error("a service must not be its own parent")
	}
	if err := db.SetServiceSetMembership(ctx, "web", ServiceSetMembership{ParentService: "db"}); err == nil {
		t.Error("parent without instance key must be refused")
	}
	if err := db.SetServiceSetMembership(ctx, "missing", ServiceSetMembership{}); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("missing service: %v", err)
	}

	// Saving the child again must keep its parent link, since SaveDesiredService does not know it.
	seedWorkloadServices(t, db, "db-0")
	if m, _ := db.GetServiceSetMembership(ctx, "db-0"); m.ParentService != "db" || m.InstanceKey != "0" {
		t.Errorf("membership lost on redeploy: %+v", m)
	}
}

func TestWorkloadInstancesChildNames(t *testing.T) {
	if got := OrderedChildName("db", 2); got != "db-2" {
		t.Errorf("ordered name %q", got)
	}
	a, b := PerNodeChildName("agent", "node-a"), PerNodeChildName("agent", "node-b")
	if len(a) != len("agent-n")+6 || a == b || a != PerNodeChildName("agent", "node-a") {
		t.Errorf("per-node names must be stable and distinct: %q %q", a, b)
	}
}

func TestWorkloadInstancesLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedWorkloadServices(t, db, "db")

	for _, ord := range []int{1, 0} {
		wi := WorkloadInstance{Service: "db", InstanceKey: fmt.Sprint(ord), ChildService: OrderedChildName("db", ord), Ordinal: ord, NodeID: "n1"}
		if err := db.UpsertWorkloadInstance(ctx, wi); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertWorkloadInstance(ctx, WorkloadInstance{Service: "db", InstanceKey: "9", ChildService: "db-9", State: "bogus"}); err == nil {
		t.Error("unknown state must be refused")
	}
	list, err := db.ListWorkloadInstances(ctx, "db")
	if err != nil || len(list) != 2 || list[0].Ordinal != 0 || list[0].Generation != 1 || list[0].State != WorkloadInstancePending {
		t.Fatalf("list: %+v %v", list, err)
	}

	if err := db.FenceWorkloadInstance(ctx, "db", "0", "disk lost"); err != nil {
		t.Fatal(err)
	}
	wi, _ := db.GetWorkloadInstanceByChild(ctx, "db-0")
	if wi.State != WorkloadInstanceFenced || wi.FencedNodeID != "n1" || wi.FencedReason != "disk lost" || wi.FencedAt.IsZero() || wi.Generation != 1 {
		t.Fatalf("fence: %+v", wi)
	}
	if err := db.SetWorkloadInstanceState(ctx, "db", "1", WorkloadInstanceFenced); err == nil {
		t.Error("fencing through SetWorkloadInstanceState must be refused")
	}

	// A repair pass must not reset the generation or fence.
	if err := db.UpsertWorkloadInstance(ctx, WorkloadInstance{Service: "db", InstanceKey: "0", ChildService: "db-0", NodeID: "n1", State: WorkloadInstanceFenced}); err != nil {
		t.Fatal(err)
	}
	if wi, _ = db.GetWorkloadInstance(ctx, "db", "0"); wi.FencedReason != "disk lost" || wi.Generation != 1 {
		t.Fatalf("upsert reset fencing: %+v", wi)
	}

	gen, err := db.MoveWorkloadInstance(ctx, "db", "0", "n2", 1)
	if err != nil || gen != 2 {
		t.Fatalf("move: %d %v", gen, err)
	}
	if _, err := db.MoveWorkloadInstance(ctx, "db", "0", "n3", 1); !errors.Is(err, ErrWorkloadInstanceStale) {
		t.Errorf("second move on the old generation: %v", err)
	}
	if _, err := db.MoveWorkloadInstance(ctx, "db", "7", "n3", 1); !errors.Is(err, ErrWorkloadInstanceNotFound) {
		t.Errorf("move of missing instance: %v", err)
	}
	wi, _ = db.GetWorkloadInstance(ctx, "db", "0")
	if wi.NodeID != "n2" || wi.State != WorkloadInstancePending || wi.FencedReason != "" || !wi.FencedAt.IsZero() {
		t.Fatalf("after move: %+v", wi)
	}

	if err := db.SetWorkloadInstanceState(ctx, "db", "0", WorkloadInstanceActive); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteWorkloadInstance(ctx, "db", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetWorkloadInstance(ctx, "db", "1"); !errors.Is(err, ErrWorkloadInstanceNotFound) {
		t.Errorf("deleted instance: %v", err)
	}
	if err := db.UpsertWorkloadInstance(ctx, WorkloadInstance{Service: "ghost", InstanceKey: "0", ChildService: "ghost-0"}); err == nil {
		t.Error("instance of an unknown parent must be refused")
	}
}
