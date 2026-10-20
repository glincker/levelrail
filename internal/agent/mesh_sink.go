package agent

// GRPCSink is the control-plane side of mesh config distribution to remote agents.
// It resolves each node's registered transport and forwards the call over that node's connection.
import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/network"
)

// meshTransport is the slice of *GRPCTransport GRPCSink needs, as an interface so tests can fake it.
type meshTransport interface {
	ApplyMesh(ctx context.Context, cfg network.DeviceConfig) (network.NodeIdentity, error)
	RotateMeshKey(ctx context.Context) (network.RotationResult, error)
}

// GRPCSink dispatches mesh config to remote nodes over their own agent
// Session stream, resolved per call through registry.
type GRPCSink struct {
	registry *Registry
}

// NewGRPCSink builds a GRPCSink that resolves node transports through registry.
func NewGRPCSink(registry *Registry) *GRPCSink {
	return &GRPCSink{registry: registry}
}

var (
	_ network.ConfigSink = (*GRPCSink)(nil)
	_ network.KeyRotator = (*GRPCSink)(nil)
)

// ErrNodeNotMeshCapable is returned when a node's registered transport does not support mesh operations. Real agent connections never hit it.
var ErrNodeNotMeshCapable = fmt.Errorf("network: node's transport does not support mesh operations")

// ApplyMesh forwards cfg to nodeID's live agent session, failing only that node's pass if it has no registered transport.
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
