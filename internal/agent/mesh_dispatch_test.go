package agent

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// fakeMeshApplier is a MeshApplier test double: it records what it was
// asked to apply and answers with canned results or errors, the same
// recording-fake shape internal/network's own recordingSink establishes
// for ConfigSink.
type fakeMeshApplier struct {
	applyErr     error
	applyResult  network.NodeIdentity
	rotateErr    error
	rotateResult network.RotationResult

	appliedCfg    network.DeviceConfig
	appliedNodeID string
	rotatedNodeID string
}

func (f *fakeMeshApplier) ApplyMesh(_ context.Context, nodeID string, cfg network.DeviceConfig) (network.NodeIdentity, error) {
	f.appliedNodeID = nodeID
	f.appliedCfg = cfg
	if f.applyErr != nil {
		return network.NodeIdentity{}, f.applyErr
	}
	return f.applyResult, nil
}

func (f *fakeMeshApplier) RotateKey(_ context.Context, nodeID string) (network.RotationResult, error) {
	f.rotatedNodeID = nodeID
	if f.rotateErr != nil {
		return network.RotationResult{}, f.rotateErr
	}
	return f.rotateResult, nil
}

func collectOneResponse(t *testing.T, fn func(send func(*agentpb.AgentMessage))) *agentpb.AgentResponse {
	t.Helper()
	var got *agentpb.AgentResponse
	fn(func(msg *agentpb.AgentMessage) {
		got = msg.GetResponse()
	})
	if got == nil {
		t.Fatal("handler never sent a response")
	}
	return got
}

func TestHandleApplyMesh_NoMeshApplier_ReturnsErrMeshUnavailable(t *testing.T) {
	req := &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{NodeId: "node-a"}}
	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleApplyMesh(context.Background(), nil, "node-a", "req-1", req, send)
	})
	if resp.GetRequestId() != "req-1" {
		t.Errorf("RequestId = %q, want %q", resp.GetRequestId(), "req-1")
	}
	if resp.GetError() != ErrMeshUnavailable.Error() {
		t.Errorf("Error = %q, want %q", resp.GetError(), ErrMeshUnavailable.Error())
	}
}

func TestHandleApplyMesh_Success(t *testing.T) {
	applier := &fakeMeshApplier{
		applyResult: network.NodeIdentity{
			PublicKey:  testMeshKey(t, 4),
			ListenPort: 51820,
			Endpoint:   "203.0.113.5:51820",
		},
	}
	req := &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{
		NodeId:  "node-a",
		Address: "10.181.0.3/16",
	}}

	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleApplyMesh(context.Background(), applier, "node-a", "req-2", req, send)
	})

	if resp.GetError() != "" {
		t.Fatalf("Error = %q, want no error", resp.GetError())
	}
	if applier.appliedNodeID != "node-a" {
		t.Errorf("ApplyMesh called with nodeID = %q, want %q", applier.appliedNodeID, "node-a")
	}
	wantAddr := netip.MustParsePrefix("10.181.0.3/16")
	if applier.appliedCfg.Address != wantAddr {
		t.Errorf("ApplyMesh called with Address = %v, want %v", applier.appliedCfg.Address, wantAddr)
	}
	got := resp.GetApplyMesh().GetIdentity()
	if got.GetPublicKey() != applier.applyResult.PublicKey.String() {
		t.Errorf("response PublicKey = %q, want %q", got.GetPublicKey(), applier.applyResult.PublicKey.String())
	}
	if got.GetEndpoint() != applier.applyResult.Endpoint {
		t.Errorf("response Endpoint = %q, want %q", got.GetEndpoint(), applier.applyResult.Endpoint)
	}
}

func TestHandleApplyMesh_ApplierError_SurfacesAsResponseError(t *testing.T) {
	wantErr := errors.New("device apply failed")
	applier := &fakeMeshApplier{applyErr: wantErr}
	req := &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{NodeId: "node-a"}}

	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleApplyMesh(context.Background(), applier, "node-a", "req-3", req, send)
	})
	if resp.GetError() != wantErr.Error() {
		t.Errorf("Error = %q, want %q", resp.GetError(), wantErr.Error())
	}
	if resp.GetApplyMesh() != nil {
		t.Error("response carried an ApplyMesh result alongside an error, want none")
	}
}

func TestHandleApplyMesh_MalformedConfig_NeverReachesApplier(t *testing.T) {
	applier := &fakeMeshApplier{}
	req := &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{
		NodeId: "node-a",
		Peers: []*agentpb.PeerConfig{{
			NodeId:     "node-b",
			PublicKey:  "not-a-key",
			AllowedIps: []string{"10.181.0.4/32"},
		}},
	}}

	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleApplyMesh(context.Background(), applier, "node-a", "req-4", req, send)
	})
	if resp.GetError() == "" {
		t.Fatal("Error = \"\", want a parse error for a malformed peer key")
	}
	if applier.appliedNodeID != "" {
		t.Error("ApplyMesh was called despite the config failing to parse")
	}
}

func TestHandleRotateMeshKey_NoMeshApplier_ReturnsErrMeshUnavailable(t *testing.T) {
	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleRotateMeshKey(context.Background(), nil, "node-a", "req-5", send)
	})
	if resp.GetError() != ErrMeshUnavailable.Error() {
		t.Errorf("Error = %q, want %q", resp.GetError(), ErrMeshUnavailable.Error())
	}
}

func TestHandleRotateMeshKey_Success(t *testing.T) {
	applier := &fakeMeshApplier{
		rotateResult: network.RotationResult{
			NodeID:       "node-a",
			OldPublicKey: testMeshKey(t, 1),
			NewPublicKey: testMeshKey(t, 2),
		},
	}
	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleRotateMeshKey(context.Background(), applier, "node-a", "req-6", send)
	})
	if resp.GetError() != "" {
		t.Fatalf("Error = %q, want no error", resp.GetError())
	}
	if applier.rotatedNodeID != "node-a" {
		t.Errorf("RotateKey called with nodeID = %q, want %q", applier.rotatedNodeID, "node-a")
	}
	got := resp.GetRotateMeshKey()
	if got.GetOldPublicKey() != testMeshKey(t, 1).String() {
		t.Errorf("OldPublicKey = %q, want %q", got.GetOldPublicKey(), testMeshKey(t, 1).String())
	}
	if got.GetNewPublicKey() != testMeshKey(t, 2).String() {
		t.Errorf("NewPublicKey = %q, want %q", got.GetNewPublicKey(), testMeshKey(t, 2).String())
	}
}

func TestHandleRotateMeshKey_ApplierError_SurfacesAsResponseError(t *testing.T) {
	wantErr := errors.New("rotate failed")
	applier := &fakeMeshApplier{rotateErr: wantErr}
	resp := collectOneResponse(t, func(send func(*agentpb.AgentMessage)) {
		handleRotateMeshKey(context.Background(), applier, "node-a", "req-7", send)
	})
	if resp.GetError() != wantErr.Error() {
		t.Errorf("Error = %q, want %q", resp.GetError(), wantErr.Error())
	}
	if resp.GetRotateMeshKey() != nil {
		t.Error("response carried a RotateMeshKey result alongside an error, want none")
	}
}
