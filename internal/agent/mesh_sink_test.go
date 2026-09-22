package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/network"
)

// fakeMeshTransport is a Transport that also implements meshTransport,
// standing in for a real *GRPCTransport reaching a connected agent. The
// embedded Transport is nil: no test here calls a docker.Runtime method
// through it, only ApplyMesh/RotateMeshKey.
type fakeMeshTransport struct {
	Transport

	applyResult  network.NodeIdentity
	applyErr     error
	rotateResult network.RotationResult
	rotateErr    error

	appliedCfg network.DeviceConfig
}

func (f *fakeMeshTransport) ApplyMesh(_ context.Context, cfg network.DeviceConfig) (network.NodeIdentity, error) {
	f.appliedCfg = cfg
	if f.applyErr != nil {
		return network.NodeIdentity{}, f.applyErr
	}
	return f.applyResult, nil
}

func (f *fakeMeshTransport) RotateMeshKey(_ context.Context) (network.RotationResult, error) {
	if f.rotateErr != nil {
		return network.RotationResult{}, f.rotateErr
	}
	return f.rotateResult, nil
}

// meshIncapableTransport is a registered Transport that does not
// implement meshTransport at all, standing in for a test double or a
// future transport kind GRPCSink's own doc comment says should never
// happen against a real agent connection but must still fail clearly.
type meshIncapableTransport struct {
	docker.Runtime
}

func TestGRPCSink_ApplyMesh_ForwardsToRegisteredTransport(t *testing.T) {
	registry := NewRegistry()
	transport := &fakeMeshTransport{applyResult: network.NodeIdentity{PublicKey: testMeshKey(t, 5)}}
	registry.Register("node-a", transport)

	sink := NewGRPCSink(registry)
	cfg := network.DeviceConfig{NodeID: "node-a"}

	got, err := sink.ApplyMesh(context.Background(), "node-a", cfg)
	if err != nil {
		t.Fatalf("ApplyMesh: %v", err)
	}
	if got.PublicKey != transport.applyResult.PublicKey {
		t.Errorf("ApplyMesh() = %+v, want %+v", got, transport.applyResult)
	}
	if transport.appliedCfg.NodeID != "node-a" {
		t.Errorf("forwarded config NodeID = %q, want %q", transport.appliedCfg.NodeID, "node-a")
	}
}

func TestGRPCSink_ApplyMesh_UnregisteredNode_ReturnsErrNodeNotRegistered(t *testing.T) {
	sink := NewGRPCSink(NewRegistry())
	_, err := sink.ApplyMesh(context.Background(), "node-missing", network.DeviceConfig{})
	if !errors.Is(err, ErrNodeNotRegistered) {
		t.Fatalf("ApplyMesh() error = %v, want ErrNodeNotRegistered", err)
	}
}

func TestGRPCSink_ApplyMesh_NotMeshCapableTransport_ReturnsErrNodeNotMeshCapable(t *testing.T) {
	registry := NewRegistry()
	registry.Register("node-a", &meshIncapableTransport{})
	sink := NewGRPCSink(registry)

	_, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{})
	if !errors.Is(err, ErrNodeNotMeshCapable) {
		t.Fatalf("ApplyMesh() error = %v, want ErrNodeNotMeshCapable", err)
	}
}

func TestGRPCSink_ApplyMesh_TransportError_Propagates(t *testing.T) {
	wantErr := errors.New("device apply failed")
	registry := NewRegistry()
	registry.Register("node-a", &fakeMeshTransport{applyErr: wantErr})
	sink := NewGRPCSink(registry)

	_, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ApplyMesh() error = %v, want %v", err, wantErr)
	}
}

func TestGRPCSink_RotateKey_ForwardsToRegisteredTransport_AndSetsNodeID(t *testing.T) {
	registry := NewRegistry()
	transport := &fakeMeshTransport{rotateResult: network.RotationResult{
		OldPublicKey: testMeshKey(t, 1),
		NewPublicKey: testMeshKey(t, 2),
		// NodeID deliberately left zero here: GRPCSink.RotateKey must set
		// it itself, since the remote node's own RotateMeshKeyResponse
		// carries no NodeID field to echo back (RotateMeshKeyRequest's
		// own doc comment: it always means "the node holding this
		// stream").
	}}
	registry.Register("node-a", transport)

	sink := NewGRPCSink(registry)
	got, err := sink.RotateKey(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if got.NodeID != "node-a" {
		t.Errorf("RotateKey() NodeID = %q, want %q", got.NodeID, "node-a")
	}
	if got.NewPublicKey != transport.rotateResult.NewPublicKey {
		t.Errorf("RotateKey() NewPublicKey = %v, want %v", got.NewPublicKey, transport.rotateResult.NewPublicKey)
	}
}

func TestGRPCSink_RotateKey_UnregisteredNode_ReturnsErrNodeNotRegistered(t *testing.T) {
	sink := NewGRPCSink(NewRegistry())
	_, err := sink.RotateKey(context.Background(), "node-missing")
	if !errors.Is(err, ErrNodeNotRegistered) {
		t.Fatalf("RotateKey() error = %v, want ErrNodeNotRegistered", err)
	}
}

func TestGRPCSink_RotateKey_NotMeshCapableTransport_ReturnsErrNodeNotMeshCapable(t *testing.T) {
	registry := NewRegistry()
	registry.Register("node-a", &meshIncapableTransport{})
	sink := NewGRPCSink(registry)

	_, err := sink.RotateKey(context.Background(), "node-a")
	if !errors.Is(err, ErrNodeNotMeshCapable) {
		t.Fatalf("RotateKey() error = %v, want ErrNodeNotMeshCapable", err)
	}
}

func TestGRPCSink_RotateKey_TransportError_Propagates(t *testing.T) {
	wantErr := errors.New("rotate failed")
	registry := NewRegistry()
	registry.Register("node-a", &fakeMeshTransport{rotateErr: wantErr})
	sink := NewGRPCSink(registry)

	_, err := sink.RotateKey(context.Background(), "node-a")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RotateKey() error = %v, want %v", err, wantErr)
	}
}

// Interface satisfaction check mirroring mesh_sink.go's own package-level
// assertions, kept here too so a reader of this test file sees the
// contract being exercised without jumping files.
var _ meshTransport = (*fakeMeshTransport)(nil)
