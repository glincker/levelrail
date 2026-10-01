package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
)

// Reproduces the bug: a single-node install with mesh enabled assigns
// auto-placed services the local node's own real mesh ID instead of the
// "" sentinel (ListNodes includes the local node as an eligible,
// schedulable, online candidate). resolveNodeTransport only special-cases
// "", so without runtimeNodeID's translation this looked up the local
// node's real ID in the remote agent registry, found nothing (only
// remote agents register there via enrollment), and the service was
// silently skipped every reconcile pass.
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

// End-to-end through resolveNodeTransport: the actual call appControllersFor/
// databaseControllersFor make, proving a service placed on the local node's
// real mesh ID resolves to the local runtime instead of erroring against an
// empty remote registry. fakeInspectRuntime (database_telemetry_targets_test.go)
// is reused as a docker.Runtime stand-in; no method on it is called here.
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
