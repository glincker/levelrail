package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
)

// Regression: auto-placed services got the local node's real mesh ID, which resolveNodeTransport does not translate, so they were silently skipped.
func TestRuntimeNodeID(t *testing.T) {
	tests := []struct {
		name    string
		meshCfg *meshSetup
		nodeID  string
		want    string
	}{
		{"no mesh, sentinel stays sentinel", nil, "", ""},
		{"no mesh, remote id passes through", nil, "remote-1", "remote-1"},
		{"mesh enabled, sentinel stays sentinel", &meshSetup{localNodeID: "local-abc"}, "", ""},
		{"mesh enabled, local node's real id becomes the sentinel", &meshSetup{localNodeID: "local-abc"}, "local-abc", ""},
		{"mesh enabled, a different node's real id passes through", &meshSetup{localNodeID: "local-abc"}, "remote-1", "remote-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := dynamicSourceDeps{meshCfg: tt.meshCfg}
			if got := runtimeNodeID(deps, tt.nodeID); got != tt.want {
				t.Errorf("runtimeNodeID(%q) = %q, want %q", tt.nodeID, got, tt.want)
			}
		})
	}
}

// Proves a service placed on the local node resolves to the local runtime, not an error against an empty remote registry.
func TestResolveNodeTransport_LocalNodeRealID_ResolvesToLocal(t *testing.T) {
	localRuntime := &fakeInspectRuntime{}
	registry := agent.NewRegistry()
	deps := dynamicSourceDeps{meshCfg: &meshSetup{localNodeID: "local-abc"}}

	rt, err := resolveNodeTransport(localRuntime, registry, runtimeNodeID(deps, "local-abc"))
	if err != nil {
		t.Fatalf("resolveNodeTransport() error = %v, want nil (should resolve to the local runtime)", err)
	}
	if rt != localRuntime {
		t.Error("resolveNodeTransport() did not return the local runtime for the local node's own real mesh id")
	}
}

// A genuinely remote, unregistered node must still fail the same way it
// always did: runtimeNodeID only special-cases this process's own id.
func TestResolveNodeTransport_UnregisteredRemoteID_StillErrors(t *testing.T) {
	localRuntime := &fakeInspectRuntime{}
	registry := agent.NewRegistry()
	deps := dynamicSourceDeps{meshCfg: &meshSetup{localNodeID: "local-abc"}}

	if _, err := resolveNodeTransport(localRuntime, registry, runtimeNodeID(deps, "remote-unregistered")); err == nil {
		t.Error("resolveNodeTransport() error = nil, want an error for a remote node with no registered transport")
	}
}
