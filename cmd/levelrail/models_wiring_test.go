package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestModelNodes_NodeInfo(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now()
	for _, id := range []string{"meshed", "unmeshed"} {
		if err := db.SaveNode(ctx, store.Node{ID: id, Name: id, Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateNodeMesh(ctx, "meshed", "pubkey", "10.99.0.2"); err != nil {
		t.Fatal(err)
	}
	info := gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0}}}
	_ = db.SetNodeGPU(ctx, store.LocalNodeGPUKey, info)
	_ = db.SetNodeGPU(ctx, "meshed", info)

	n := modelNodes{db: db, localNodeID: "self"}
	tests := []struct {
		name     string
		nodeID   string
		wantBind string
		wantDial string
		wantGPU  bool
		wantErr  bool
	}{
		{name: "local sentinel", nodeID: "", wantBind: "127.0.0.1", wantDial: "127.0.0.1", wantGPU: true},
		{name: "local by id", nodeID: "self", wantBind: "127.0.0.1", wantDial: "127.0.0.1", wantGPU: true},
		{name: "remote on mesh", nodeID: "meshed", wantBind: "10.99.0.2", wantDial: "10.99.0.2", wantGPU: true},
		{name: "remote off mesh", nodeID: "unmeshed", wantBind: "127.0.0.1", wantDial: ""},
		{name: "unknown remote", nodeID: "ghost", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := n.NodeInfo(ctx, tt.nodeID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.BindIP != tt.wantBind || got.DialHost != tt.wantDial || got.GPU.Present != tt.wantGPU {
				t.Errorf("NodeInfo = %+v", got)
			}
		})
	}
	if g, err := n.NodeGPU(ctx, "unmeshed"); err != nil || g.Present {
		t.Errorf("NodeGPU of a node without a snapshot = %+v, %v", g, err)
	}
}
