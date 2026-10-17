package agent

// Agent-side ApplyMesh/RotateMeshKey dispatch, called from serveSession rather than Execute.
// Execute would need this node's ID and a MeshApplier for two ops, so they stay out of its signature.

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// MeshApplier is satisfied by *network.LocalSink's ApplyMesh and RotateKey methods, with no adapter.
type MeshApplier interface {
	ApplyMesh(ctx context.Context, nodeID string, cfg network.DeviceConfig) (network.NodeIdentity, error)
	RotateKey(ctx context.Context, nodeID string) (network.RotationResult, error)
}

// ErrMeshUnavailable is returned when no MeshApplier was wired in.
// The control plane treats it as a per-node failure, never a fleet-wide one.
var ErrMeshUnavailable = errors.New("agent: this node has no mesh networking configured")

// handleApplyMesh applies this node's mesh config and reports the values only this node knows.
func handleApplyMesh(ctx context.Context, mesh MeshApplier, nodeID, requestID string, req *agentpb.ApplyMeshRequest, send func(*agentpb.AgentMessage)) {
	resp := &agentpb.AgentResponse{RequestId: requestID}
	defer func() { send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}}) }()

	if mesh == nil {
		resp.Error = ErrMeshUnavailable.Error()
		return
	}
	cfg, err := deviceConfigFromPB(req.GetConfig())
	if err != nil {
		resp.Error = err.Error()
		return
	}
	identity, err := mesh.ApplyMesh(ctx, nodeID, cfg)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Result = &agentpb.AgentResponse_ApplyMesh{ApplyMesh: &agentpb.ApplyMeshResponse{Identity: nodeIdentityToPB(identity)}}
}

// handleRotateMeshKey generates a fresh keypair, goes live on it immediately, and reports the new key.
func handleRotateMeshKey(ctx context.Context, mesh MeshApplier, nodeID, requestID string, send func(*agentpb.AgentMessage)) {
	resp := &agentpb.AgentResponse{RequestId: requestID}
	defer func() { send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}}) }()

	if mesh == nil {
		resp.Error = ErrMeshUnavailable.Error()
		return
	}
	result, err := mesh.RotateKey(ctx, nodeID)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Result = &agentpb.AgentResponse_RotateMeshKey{RotateMeshKey: &agentpb.RotateMeshKeyResponse{
		OldPublicKey: result.OldPublicKey.String(),
		NewPublicKey: result.NewPublicKey.String(),
	}}
}
