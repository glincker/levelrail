package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeNodeTransport is a minimal docker.Runtime plus docker.StatsInspector
// fake, standing in for a remote node's agent.Transport the way
// fakeInspectRuntime (database_telemetry_targets_test.go) stands in for a
// local docker.Runtime: only the methods telemetryTargets/
// multiNodeStatsSource actually call are implemented.
type fakeNodeTransport struct {
	fakeInspectRuntime
	statsCalls []string
}

func (f *fakeNodeTransport) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	return []docker.ContainerState{{ID: "remote-container", Name: prefix + "abc123", Running: true}}, nil
}

func (f *fakeNodeTransport) Stats(_ context.Context, containerID string) (docker.ContainerStats, error) { //nolint:unparam // docker.StatsInspector's signature, not this fake's choice
	f.statsCalls = append(f.statsCalls, containerID)
	return docker.ContainerStats{MemoryUsageBytes: 999}, nil
}

// TestTelemetryTargets_RemoteService_RoutesThroughRegisteredAgent proves
// the actual gap this change closes: a service placed on a remote node
// (store.DesiredService.NodeID set via UpdateServiceNode) is discovered
// through that node's own registered Transport, not the control plane's
// local Docker client, and tagged with the right Target.NodeID for
// multiNodeStatsSource to route its stats poll correctly too.
func TestTelemetryTargets_RemoteService_RoutesThroughRegisteredAgent(t *testing.T) {
	db := openCredentialsTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed service web: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node-2"); err != nil {
		t.Fatalf("place web on node-2: %v", err)
	}

	remote := &fakeNodeTransport{}
	registry := agent.NewRegistry()
	registry.Register("node-2", remote)

	local := &fakeInspectRuntime{} // never used: web is placed on node-2, not local

	targetsFunc := telemetryTargets(db, local, registry, nil, slog.Default())
	targets, err := targetsFunc(ctx)
	if err != nil {
		t.Fatalf("telemetryTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %+v, want exactly 1 (web's remote container)", targets)
	}
	got := targets[0]
	if got.ResourceID != "service:web" || got.ContainerID != "remote-container" || got.NodeID != "node-2" {
		t.Errorf("targets[0] = %+v, want ResourceID=service:web ContainerID=remote-container NodeID=node-2", got)
	}

	source := &multiNodeStatsSource{local: local, registry: registry}
	stats, err := source.Stats(ctx, got.NodeID, got.ContainerID)
	if err != nil {
		t.Fatalf("multiNodeStatsSource.Stats() error = %v", err)
	}
	if stats.MemoryUsageBytes != 999 {
		t.Errorf("Stats() = %+v, want the remote node's own sample (999), not a local one", stats)
	}
	if len(remote.statsCalls) != 1 || remote.statsCalls[0] != "remote-container" {
		t.Errorf("remote.statsCalls = %+v, want exactly one call for remote-container", remote.statsCalls)
	}
}

// TestTelemetryTargets_DisconnectedRemoteNode_SkipsServiceNotWholeTick
// proves a service on an unreachable node doesn't take every other
// service's metrics down with it this tick (resolveNodeTransport's own
// "one broken resource must not block others" precedent).
func TestTelemetryTargets_DisconnectedRemoteNode_SkipsServiceNotWholeTick(t *testing.T) {
	db := openCredentialsTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "offline", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed service offline: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "offline", "node-unreachable"); err != nil {
		t.Fatalf("place offline on node-unreachable: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "local", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed service local: %v", err)
	}

	local := &fakeNodeTransport{}
	registry := agent.NewRegistry() // node-unreachable deliberately never registered

	targetsFunc := telemetryTargets(db, local, registry, nil, slog.Default())
	targets, err := targetsFunc(ctx)
	if err != nil {
		t.Fatalf("telemetryTargets() error = %v, want the unreachable node's error swallowed, not propagated", err)
	}
	if len(targets) != 1 || targets[0].ResourceID != "service:local" {
		t.Errorf("targets = %+v, want exactly local's container, offline's skipped", targets)
	}
}
