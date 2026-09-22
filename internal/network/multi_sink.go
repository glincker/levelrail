package network

// This file: MultiSink, the ConfigSink a control plane actually hands
// its Coordinator once remote distribution exists. A Coordinator holds
// exactly one sink (NewCoordinator's own signature), but a fleet has two
// kinds of node from this process's point of view: the control plane's
// own local node (LocalSink, in-process, no network) and every other,
// remote node (a gRPC-backed sink, over the agent Session stream this
// package deliberately does not implement, per ConfigSink's own doc
// comment). MultiSink is the seam that lets both share one Coordinator:
// it knows nothing about gRPC or any other transport, only "one node ID
// is local, route everything else elsewhere," so the actual remote
// implementation (internal/agent.GRPCSink) plugs in as an ordinary
// ConfigSink without this package importing internal/agent, keeping the
// layering internal/agent already established (agent depends on network,
// never the reverse).

import (
	"context"
	"fmt"
)

// MultiSink dispatches ApplyMesh/RotateKey by node ID: localNodeID goes
// to Local, everything else goes to Remote.
type MultiSink struct {
	localNodeID string
	local       ConfigSink
	remote      ConfigSink
}

// NewMultiSink builds a MultiSink. remote may be nil, for a control plane
// that has not wired up remote distribution at all (the pre-this-change
// behavior): a config addressed to any node but localNodeID then fails
// with a clear per-node error instead of a nil-pointer panic, the same
// "a missing capability is a clear error, not a crash" contract
// ErrRotationNotSupported already gives a different missing capability.
func NewMultiSink(localNodeID string, local, remote ConfigSink) *MultiSink {
	return &MultiSink{localNodeID: localNodeID, local: local, remote: remote}
}

var (
	_ ConfigSink = (*MultiSink)(nil)
	_ KeyRotator = (*MultiSink)(nil)
)

// ApplyMesh implements ConfigSink, routing by nodeID.
func (m *MultiSink) ApplyMesh(ctx context.Context, nodeID string, cfg DeviceConfig) (NodeIdentity, error) {
	if nodeID == m.localNodeID {
		return m.local.ApplyMesh(ctx, nodeID, cfg)
	}
	if m.remote == nil {
		return NodeIdentity{}, fmt.Errorf("network: apply mesh: no remote mesh sink configured for node %q", nodeID)
	}
	return m.remote.ApplyMesh(ctx, nodeID, cfg)
}

// RotateKey implements KeyRotator, routing by nodeID the same way
// ApplyMesh does. A sink that does not itself implement KeyRotator (a
// read-only or test sink, per KeyRotator's own doc comment on why it is
// separate from ConfigSink) makes RotateKey fail with
// ErrRotationNotSupported for that half of the fleet only, exactly the
// same per-node granularity ApplyMesh already has via NodeResult.Err.
func (m *MultiSink) RotateKey(ctx context.Context, nodeID string) (RotationResult, error) {
	sink, target := m.local, "local"
	if nodeID != m.localNodeID {
		sink, target = m.remote, "remote"
	}
	if sink == nil {
		return RotationResult{}, fmt.Errorf("network: rotate key: no %s mesh sink configured for node %q", target, nodeID)
	}
	rotator, ok := sink.(KeyRotator)
	if !ok {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: %w", nodeID, ErrRotationNotSupported)
	}
	return rotator.RotateKey(ctx, nodeID)
}
