package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/network"
)

// The embedded Transport is nil, so only ApplyMesh and RotateMeshKey may be called on it.
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

// Exercises the failure path for a transport without mesh support, which a real agent never produces.
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
		// RotateKey must set NodeID itself, since the node's response carries no NodeID to echo back.
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
