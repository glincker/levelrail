package ingress

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/experimental"
	"github.com/glincker/levelrail/internal/meshpath"
	"github.com/glincker/levelrail/internal/store"
)

func TestCrossNodeStatus_TunnelHints(t *testing.T) {
	tests := []struct {
		name      string
		connected bool
		path      meshpath.Path
		want      string
		notWant   string
	}{
		{name: "flag on, agent lacks capability", connected: true, path: meshpath.Path{NoMesh: true, Reason: "node has not joined the mesh yet"}, want: "upgrade the agent and set APP_EXPERIMENTAL=agent-ingress-tunnel"},
		{name: "offline no-mesh node", path: meshpath.Path{NoMesh: true, Reason: "node has not joined the mesh yet"}, want: "offline", notWant: "APP_MESH_ENABLED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			experimental.Set(experimental.AgentIngressTunnel)
			t.Cleanup(experimental.Reset)
			tun := newTestTunnels(t, false)
			tun.Connected = func(string) bool { return tt.connected }
			c := New(&fakeStore{services: []store.DesiredService{remoteSvc()}}, newFakeRuntime(), &fakeApplier{},
				WithLogger(slog.New(slog.DiscardHandler)), WithLocalNodeID("node-1"),
				WithMeshPaths(&fakePaths{path: tt.path}), WithAgentTunnels(tun))
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cond := crossNodeCond(res)
			if cond == nil || cond.Reason != "NoMeshIngressPath" {
				t.Fatalf("condition = %+v, want NoMeshIngressPath", cond)
			}
			if !strings.Contains(cond.Message, tt.want) || (tt.notWant != "" && strings.Contains(cond.Message, tt.notWant)) {
				t.Fatalf("message = %q, want %q and not %q", cond.Message, tt.want, tt.notWant)
			}
		})
	}
}
