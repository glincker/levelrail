package agent

// This file: GRPCSink, the control-plane side of mesh config
// distribution to a real, remote agent. This is exactly the piece
// internal/network.ConfigSink's own doc comment describes as
// "deliberately not in this change" (network/plan.go's header) and
// cmd/levelrail/mesh.go's top-of-file note names as the reason
// APP_MESH_ENABLED only brings up the control plane's own node today:
// "internal/network.ConfigSink's gRPC arm... does not exist yet."
// It now does.
//
// GRPCSink implements network.ConfigSink and network.KeyRotator, but
// (unlike network.LocalSink) is not itself the transport: it holds a
// *Registry and, for each call, resolves the Transport a real agent's
// Session registered for that node ID, then forwards the call over that
// specific connection. One GRPCSink instance is shared across every
// remote node in the fleet; the *mux underneath each resolved Transport
// is what's actually per-node. See meshTransport for the narrow slice of
// GRPCTransport this needs, and network.MultiSink for how this and
// LocalSink (which only ever serves the control plane's own local node)
// combine into the one ConfigSink a Coordinator holds.
import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/network"
)

// meshTransport is the slice of *GRPCTransport GRPCSink needs, extracted
// as an interface for the same reason agentClientStream and sessionStream
// already are in this package: a caller can fake it in a test without a
// real Session stream. *GRPCTransport satisfies this structurally (see
// its ApplyMesh/RotateMeshKey methods below).
type meshTransport interface {
	ApplyMesh(ctx context.Context, cfg network.DeviceConfig) (network.NodeIdentity, error)
	RotateMeshKey(ctx context.Context) (network.RotationResult, error)
}

// GRPCSink dispatches mesh config to remote nodes over their own agent
// Session stream, resolved per call through registry.
type GRPCSink struct {
	registry *Registry
}

// NewGRPCSink builds a GRPCSink that resolves each node's transport
// through registry, the same *Registry Server.Session already populates
// (server.go) and every other remote-dispatch path in this package
// (GRPCTransport itself, build dispatch) already shares.
func NewGRPCSink(registry *Registry) *GRPCSink {
	return &GRPCSink{registry: registry}
}

var (
	_ network.ConfigSink = (*GRPCSink)(nil)
	_ network.KeyRotator = (*GRPCSink)(nil)
)

// ErrNodeNotMeshCapable is returned when nodeID's registered transport
// exists (the node is connected) but does not support mesh operations,
// which in practice means it isn't really a *GRPCTransport at all (a test
// double registered directly, or a future transport kind that never
// implements mesh). Never expected against a real agent connection, since
// every real one is built by newGRPCTransport.
var ErrNodeNotMeshCapable = fmt.Errorf("network: node's transport does not support mesh operations")

// ApplyMesh implements network.ConfigSink by forwarding cfg to nodeID's
// live agent Session, if it has one. A node with no registered transport
// (never enrolled, or disconnected) fails this one node's pass with
// agent.ErrNodeNotRegistered wrapped, exactly the "one node's problem,
// never the fleet's" contract ConfigSink.ApplyMesh's own doc comment
// requires: Distribute records it in that node's NodeResult.Err and
// keeps distributing to every other node.
func (s *GRPCSink) ApplyMesh(ctx context.Context, nodeID string, cfg network.DeviceConfig) (network.NodeIdentity, error) {
	t, err := s.registry.Get(nodeID)
	if err != nil {
		return network.NodeIdentity{}, err
	}
	mt, ok := t.(meshTransport)
	if !ok {
		return network.NodeIdentity{}, fmt.Errorf("%w: node %q", ErrNodeNotMeshCapable, nodeID)
	}
	return mt.ApplyMesh(ctx, cfg)
}

// RotateKey implements network.KeyRotator by asking nodeID's live agent
// Session to rotate its own key. Same not-registered/not-mesh-capable
// failure modes as ApplyMesh.
func (s *GRPCSink) RotateKey(ctx context.Context, nodeID string) (network.RotationResult, error) {
	t, err := s.registry.Get(nodeID)
	if err != nil {
		return network.RotationResult{}, err
	}
	mt, ok := t.(meshTransport)
	if !ok {
		return network.RotationResult{}, fmt.Errorf("%w: node %q", ErrNodeNotMeshCapable, nodeID)
	}
	result, err := mt.RotateMeshKey(ctx)
	if err != nil {
		return network.RotationResult{}, err
	}
	result.NodeID = nodeID
	return result, nil
}
