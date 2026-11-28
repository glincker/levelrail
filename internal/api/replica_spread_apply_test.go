package api

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func spreadSvc(name string, replicas int) store.DesiredService {
	return store.DesiredService{Name: name, Image: "nginx:1", Port: 80, Replicas: replicas, BindAddress: "public"}
}

func enableSpread(t *testing.T, rt *Router, on string) {
	t.Helper()
	if _, err := rt.runtimeSettings.Set(context.Background(), KeyPlacementReplicaSpread, json.RawMessage(on), "test"); err != nil {
		t.Fatal(err)
	}
}

func countNodes(nodes []string) map[string]int {
	m := map[string]int{}
	for _, n := range nodes {
		m[n]++
	}
	return m
}

func TestApplyReplicaSpread_TaintedNodeGetsNone(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	seedMNNode(t, db, "n3", nil)
	if err := db.SetNodeTaints(ctx, "n3", []store.NodeTaint{{Key: "gpu", Effect: "NoSchedule"}}); err != nil {
		t.Fatal(err)
	}
	enableSpread(t, rt, "true")
	svc := spreadSvc("web", 3)
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	got, why := rt.ApplyReplicaSpread(ctx, svc)
	if why != "" || len(got.ReplicaNodes) != 3 {
		t.Fatalf("plan = %v refusal %q", got.ReplicaNodes, why)
	}
	c := countNodes(got.ReplicaNodes)
	if c["n3"] != 0 || c["n1"]+c["n2"] != 3 || (c["n1"] != 2 && c["n2"] != 2) {
		t.Fatalf("want 2 and 1 over n1,n2 only, got %v", c)
	}
	stored, err := db.GetDesiredService(ctx, "web")
	if err != nil || !slices.Equal(stored.ReplicaNodes, got.ReplicaNodes) {
		t.Fatalf("not persisted: %v %v", stored.ReplicaNodes, err)
	}
}

func TestApplyReplicaSpread_NamedVolumeRefuses(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	enableSpread(t, rt, "true")
	svc := spreadSvc("data", 2)
	svc.Volumes = []store.ServiceVolume{{Name: "d", ContainerPath: "/data"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	got, why := rt.ApplyReplicaSpread(ctx, svc)
	if why == "" || got.ReplicaNodes != nil {
		t.Fatalf("want refusal and no nodes, got %v %q", got.ReplicaNodes, why)
	}
}

func TestApplyReplicaSpread_SettingOffChangesNothing(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	svc := spreadSvc("web", 2)
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	got, why := rt.ApplyReplicaSpread(ctx, svc)
	if why != "" || got.ReplicaNodes != nil {
		t.Fatalf("off must be a no-op, got %v %q", got.ReplicaNodes, why)
	}
	stored, _ := db.GetDesiredService(ctx, "web")
	if stored.ReplicaNodes != nil {
		t.Fatalf("stored changed: %v", stored.ReplicaNodes)
	}
}

func TestApplyReplicaSpread_NodeLossReplans(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	seedMNNode(t, db, "n3", nil)
	enableSpread(t, rt, "true")
	svc := spreadSvc("web", 3)
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	first, why := rt.ApplyReplicaSpread(ctx, svc)
	if why != "" || len(countNodes(first.ReplicaNodes)) != 3 {
		t.Fatalf("want one replica per node, got %v %q", first.ReplicaNodes, why)
	}
	if err := db.UpdateNodeStatus(ctx, "n3", store.NodeStatusOffline); err != nil {
		t.Fatal(err)
	}
	second, why := rt.ApplyReplicaSpread(ctx, first)
	if why != "" || countNodes(second.ReplicaNodes)["n3"] != 0 || len(second.ReplicaNodes) != 3 {
		t.Fatalf("lost node must be replanned away, got %v %q", second.ReplicaNodes, why)
	}
}
