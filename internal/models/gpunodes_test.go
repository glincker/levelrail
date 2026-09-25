package models

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

func gpuInfo(n int, runtime bool) gpu.Info {
	info := gpu.Info{Present: n > 0, RuntimeInstalled: runtime}
	for i := 0; i < n; i++ {
		info.Devices = append(info.Devices, gpu.Device{Index: i, UUID: "GPU-" + string(rune('a'+i)), VRAMTotalMiB: 24576})
	}
	return info
}

func addNode(t *testing.T, db *store.DB, id string, info gpu.Info, schedulable bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := db.SaveNode(ctx, store.Node{ID: id, Name: id, Status: store.NodeStatusOnline, AcceptsAppWorkloads: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save node: %v", err)
	}
	if !schedulable {
		if err := db.SetNodeSchedulable(ctx, id, false); err != nil {
			t.Fatalf("cordon: %v", err)
		}
	}
	if err := db.SetNodeGPU(ctx, id, info); err != nil {
		t.Fatalf("set gpu: %v", err)
	}
}

func addGPUApp(t *testing.T, db *store.DB, name, nodeID string, g store.ServiceGPU) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "x", Port: 80, Resources: &store.ServiceResources{GPU: &g}}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, name, nodeID); err != nil {
		t.Fatalf("place service: %v", err)
	}
}

func findNode(t *testing.T, nodes []GPUNode, name string) GPUNode {
	t.Helper()
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("node %q not listed", name)
	return GPUNode{}
}

func TestGPUNodesReservations(t *testing.T) {
	ctx := context.Background()
	svc, db := newSvc(t, nil)
	addNode(t, db, "n1", gpuInfo(2, true), true)
	addGPUApp(t, db, "trainer", "n1", store.ServiceGPU{Count: 1})
	if err := db.SaveModel(ctx, store.Model{Name: "chat", Engine: EngineOllama, ModelRef: "m", NodeID: "n1", GPUDeviceIDs: []string{"1"}}); err != nil {
		t.Fatalf("save model: %v", err)
	}

	nodes, err := svc.GPUNodes(ctx)
	if err != nil {
		t.Fatalf("GPUNodes: %v", err)
	}
	n := findNode(t, nodes, "n1")
	l := n.Ledger("")
	if l.Total() != 2 || l.Reserved() != 2 || l.Free() != 0 {
		t.Fatalf("total/reserved/free = %d/%d/%d, want 2/2/0", l.Total(), l.Reserved(), l.Free())
	}
	if n.ModelCount != 1 || !n.Eligible() {
		t.Fatalf("ModelCount=%d Eligible=%v", n.ModelCount, n.Eligible())
	}
	if l2 := n.Ledger(ModelClaimName("chat")); l2.Free() != 1 {
		t.Fatalf("free excluding chat = %d, want 1", l2.Free())
	}
}

func TestUnplacedWorkloads(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T, db *store.DB)
		want  []string
	}{
		{"fits its own node", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(1, true), true)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{Count: 1})
		}, nil},
		{"runtime missing and no other node", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(1, false), true)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{Count: 1})
		}, []string{"app/a"}},
		{"runtime missing but another node has room", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(1, false), true)
			addNode(t, db, "n2", gpuInfo(1, true), true)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{Count: 1})
		}, nil},
		{"other node cordoned", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(1, false), true)
			addNode(t, db, "n2", gpuInfo(1, true), false)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{Count: 1})
		}, []string{"app/a"}},
		{"other node exactly full", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(1, false), true)
			addNode(t, db, "n2", gpuInfo(1, true), true)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{Count: 1})
			addGPUApp(t, db, "b", "n2", store.ServiceGPU{Count: 1})
		}, []string{"app/a"}},
		{"device id overlap on the only node", func(t *testing.T, db *store.DB) {
			addNode(t, db, "n1", gpuInfo(2, true), true)
			addGPUApp(t, db, "a", "n1", store.ServiceGPU{DeviceIDs: []string{"0"}})
			addGPUApp(t, db, "b", "n1", store.ServiceGPU{DeviceIDs: []string{"0"}})
		}, []string{"app/a", "app/b"}},
		{"model on a node with no snapshot", func(t *testing.T, db *store.DB) {
			if err := db.SaveModel(ctx, store.Model{Name: "m", Engine: EngineOllama, ModelRef: "x", GPUCount: 1}); err != nil {
				t.Fatalf("save model: %v", err)
			}
		}, []string{"model/m"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := newSvc(t, nil)
			tt.setup(t, db)
			got, err := svc.UnplacedWorkloads(ctx)
			if err != nil {
				t.Fatalf("UnplacedWorkloads: %v", err)
			}
			var names []string
			for _, u := range got {
				names = append(names, u.Kind+"/"+u.Name)
			}
			if len(names) != len(tt.want) {
				t.Fatalf("unplaced = %v, want %v", names, tt.want)
			}
			for i := range names {
				if names[i] != tt.want[i] {
					t.Fatalf("unplaced = %v, want %v", names, tt.want)
				}
			}
		})
	}
}
