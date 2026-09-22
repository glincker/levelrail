package network

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestLocalSink_RotateKey(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	oldPub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	mesh := NewDisabled("test")
	sink, err := NewLocalSink("self", mesh, priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	// A config with a real peer, applied first, so RotateKey has
	// something real to reapply.
	cfg := DeviceConfig{
		NodeID:  "self",
		Address: netip.MustParsePrefix("10.181.0.1/16"),
		Peers:   []PeerConfig{{NodeID: "a", PublicKey: testKey(1)}},
	}
	if _, err := sink.ApplyMesh(context.Background(), "self", cfg); err != nil {
		t.Fatalf("ApplyMesh: %v", err)
	}

	result, err := sink.RotateKey(context.Background(), "self")
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if result.OldPublicKey != oldPub {
		t.Errorf("OldPublicKey = %v, want %v", result.OldPublicKey, oldPub)
	}
	if result.NewPublicKey == oldPub {
		t.Error("RotateKey did not actually change the public key")
	}
	if result.NewPublicKey.IsZero() {
		t.Error("RotateKey returned a zero new public key")
	}
	if sink.PublicKey() != result.NewPublicKey {
		t.Error("PublicKey() disagrees with the rotation result")
	}

	// The peer set from the last applied config must have survived the
	// rotation: RotateKey reapplies lastCfg, not an empty one.
	st, err := mesh.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st.Peers) != 1 || st.Peers[0].NodeID != "a" {
		t.Fatalf("peers after rotation = %+v, want the pre-rotation peer set preserved", st.Peers)
	}
}

func TestLocalSink_RotateKey_RefusesAnotherNode(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	sink, err := NewLocalSink("self", NewDisabled("test"), priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	if _, err := sink.RotateKey(context.Background(), "somebody-else"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrUnknownNode)
	}
}

func TestLocalSink_RotateKey_PersistFailureLeavesOldKeyLive(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	oldPub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	persistErr := errors.New("disk full")
	sink, err := NewLocalSink("self", NewDisabled("test"), priv,
		WithKeyPersistFunc(func(Key) error { return persistErr }))
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	if _, err := sink.RotateKey(context.Background(), "self"); !errors.Is(err, persistErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, persistErr)
	}
	// The device must not have been touched: a key that failed to persist
	// must never go live, or a restart strands the node on a key its own
	// file disagrees with.
	if sink.PublicKey() != oldPub {
		t.Error("the public key changed despite a persist failure")
	}
}

func TestLocalSink_RotateKey_ApplyFailureLeavesOldKeyLive(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	oldPub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	mesh := NewDisabled("test")
	if err := mesh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	sink, err := NewLocalSink("self", mesh, priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	if _, err := sink.RotateKey(context.Background(), "self"); !errors.Is(err, ErrMeshClosed) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrMeshClosed)
	}
	if sink.PublicKey() != oldPub {
		t.Error("the public key changed despite the device rejecting the apply")
	}
}

// TestCoordinator_RotateKey_TracksConfirmation is the property the whole
// feature exists for: a rotation starts unconfirmed, and the very next
// clean Distribute pass confirms it, while a pass with a failure leaves
// it unconfirmed until a clean one comes along.
func TestCoordinator_RotateKey_TracksConfirmation(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	mesh := NewDisabled("test")
	localSink, err := NewLocalSink("self", mesh, priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	sink := &rotatingSink{LocalSink: localSink, failNext: map[string]bool{}}
	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))

	inventory := []NodeInfo{node("self", 1, "10.181.0.1", "")}
	if _, _, err := c.Distribute(context.Background(), inventory); err != nil {
		t.Fatalf("initial Distribute: %v", err)
	}

	result, err := c.RotateKey(context.Background(), "self")
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	status, ok := c.RotationStatusFor("self")
	if !ok {
		t.Fatal("no rotation status recorded")
	}
	if status.Confirmed {
		t.Error("rotation reported confirmed before any Distribute pass ran")
	}
	if status.NewPublicKey != result.NewPublicKey {
		t.Errorf("tracked NewPublicKey = %v, want %v", status.NewPublicKey, result.NewPublicKey)
	}

	// A pass with a failure must not confirm the rotation.
	sink.failNext["self"] = true
	if _, _, err := c.Distribute(context.Background(), inventory); err != nil {
		t.Fatalf("Distribute (failing): %v", err)
	}
	status, _ = c.RotationStatusFor("self")
	if status.Confirmed {
		t.Error("rotation confirmed despite a failed pass")
	}

	// A clean pass confirms it.
	if _, _, err := c.Distribute(context.Background(), inventory); err != nil {
		t.Fatalf("Distribute (clean): %v", err)
	}
	status, _ = c.RotationStatusFor("self")
	if !status.Confirmed {
		t.Error("rotation did not confirm after a clean pass")
	}
	if status.ConfirmedAt.IsZero() {
		t.Error("ConfirmedAt was never set")
	}

	statuses := c.RotationStatuses()
	if len(statuses) != 1 || statuses[0].NodeID != "self" {
		t.Fatalf("RotationStatuses = %+v, want exactly one entry for self", statuses)
	}
}

func TestCoordinator_RotateKey_NotSupported(t *testing.T) {
	c := NewCoordinator(newRecordingSink(), PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	if _, err := c.RotateKey(context.Background(), "a"); !errors.Is(err, ErrRotationNotSupported) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrRotationNotSupported)
	}
}

func TestCoordinator_RotateKey_RespectsContextCancellation(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	sink, err := NewLocalSink("self", NewDisabled("test"), priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.RotateKey(ctx, "self"); !errors.Is(err, context.Canceled) {
		t.Errorf("RotateKey = %v, want it to wrap context.Canceled", err)
	}
}

// rotatingSink wraps a *LocalSink so its ApplyMesh can be made to fail on
// demand, the way recordingSink's own failures map lets Coordinator tests
// simulate an unreachable node, but still backed by a real LocalSink so
// RotateKey has something real to call.
type rotatingSink struct {
	*LocalSink
	failNext map[string]bool
}

func (s *rotatingSink) ApplyMesh(ctx context.Context, nodeID string, cfg DeviceConfig) (NodeIdentity, error) {
	if s.failNext[nodeID] {
		delete(s.failNext, nodeID)
		return NodeIdentity{}, errors.New("simulated unreachable node")
	}
	return s.LocalSink.ApplyMesh(ctx, nodeID, cfg)
}
