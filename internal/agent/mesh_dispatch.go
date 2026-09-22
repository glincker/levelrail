package agent

// This file: the agent-side dispatch for ApplyMesh/RotateMeshKey
// requests, called directly from serveSession's own dispatch loop
// (client.go) rather than through Execute, the same "does not fit
// Execute's shape" reasoning already applied to Exec/Build there. Unlike
// Exec/Build this is not a multi-frame relay, it is a single request and
// a single response; what still keeps it out of Execute is that it needs
// this node's own ID and a MeshApplier, neither of which docker.Runtime
// (Execute's other parameter) carries, and threading both through
// Execute's signature for two ops out of twenty would touch every one of
// Execute's other 15+ call sites for no benefit to them.

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// MeshApplier is what serveSession needs to apply this node's own mesh
// config and rotate its own mesh key. Exactly *network.LocalSink's own
// two methods (ApplyMesh, RotateKey): a real WireGuard device wired up by
// cmd/levelrail-agent satisfies this by constructing a *network.LocalSink
// around its own network.Mesh and private key, with zero adapter code
// needed, the identical reuse the control plane's own setupMesh
// (cmd/levelrail/mesh.go) already gets from LocalSink for its own local
// node. See RunSession's WithMesh option for how this and this node's own
// ID reach serveSession.
type MeshApplier interface {
	ApplyMesh(ctx context.Context, nodeID string, cfg network.DeviceConfig) (network.NodeIdentity, error)
	RotateKey(ctx context.Context, nodeID string) (network.RotationResult, error)
}

// ErrMeshUnavailable is what an ApplyMesh/RotateMeshKey request gets when
// this node's Session was started with no MeshApplier wired up
// (RunSession's own WithMesh option never called): mesh networking not
// enabled for this node, or this agent process predates mesh support. The
// control plane's Coordinator treats this the same as any other per-node
// ApplyMesh failure (NodeResult.Err), never a fleet-wide error.
var ErrMeshUnavailable = errors.New("agent: this node has no mesh networking configured")

// handleApplyMesh answers one ApplyMeshRequest: converge this node's own
// device on the config, and report back what only this node can know
// (its public key, the port it actually bound, its own view of its
// endpoint).
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

// handleRotateMeshKey answers one RotateMeshKeyRequest: generate and go
// live on a fresh keypair for this node immediately
// (network.LocalSink.RotateKey's own doc comment), and report the key
// change.
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
