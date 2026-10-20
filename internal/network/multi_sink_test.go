package network

import (
	"context"
	"errors"
	"testing"
)

// readOnlySink is a ConfigSink that does not also implement KeyRotator,
// the same "read-only or test sink" shape MultiSink.RotateKey's own doc
// comment calls out.
type readOnlySink struct {
	*recordingSink
}

func TestMultiSink_ApplyMesh_RoutesLocalAndRemote(t *testing.T) {
	local := newRecordingSink()
	local.identities["a"] = NodeIdentity{PublicKey: testKey(1)}
	remote := newRecordingSink()
	remote.identities["b"] = NodeIdentity{PublicKey: testKey(2)}

	m := NewMultiSink("a", local, remote)

	gotLocal, err := m.ApplyMesh(context.Background(), "a", DeviceConfig{NodeID: "a"})
	if err != nil {
		t.Fatalf("ApplyMesh(a): %v", err)
	}
	if gotLocal.PublicKey != testKey(1) {
		t.Fatalf("ApplyMesh(a) identity = %+v, want local sink's answer", gotLocal)
	}
	if _, ok := local.received["a"]; !ok {
		t.Fatal("local sink never received node a's config")
	}
	if _, ok := remote.received["a"]; ok {
		t.Fatal("remote sink unexpectedly received node a's config")
	}

	gotRemote, err := m.ApplyMesh(context.Background(), "b", DeviceConfig{NodeID: "b"})
	if err != nil {
		t.Fatalf("ApplyMesh(b): %v", err)
	}
	if gotRemote.PublicKey != testKey(2) {
		t.Fatalf("ApplyMesh(b) identity = %+v, want remote sink's answer", gotRemote)
	}
	if _, ok := remote.received["b"]; !ok {
		t.Fatal("remote sink never received node b's config")
	}
	if _, ok := local.received["b"]; ok {
		t.Fatal("local sink unexpectedly received node b's config")
	}
}

func TestMultiSink_ApplyMesh_NilRemote_FailsClearlyPerNode(t *testing.T) {
	local := newRecordingSink()
	m := NewMultiSink("a", local, nil)

	_, err := m.ApplyMesh(context.Background(), "b", DeviceConfig{NodeID: "b"})
	if err == nil {
		t.Fatal("ApplyMesh() error = nil, want an error for an unconfigured remote sink")
	}
}

func TestMultiSink_RotateKey_RoutesLocalAndRemote(t *testing.T) {
	local := &recordingRotatorSink{recordingSink: newRecordingSink()}
	local.rotations = map[string]RotationResult{"a": {NodeID: "a", NewPublicKey: testKey(9)}}
	remote := &recordingRotatorSink{recordingSink: newRecordingSink()}
	remote.rotations = map[string]RotationResult{"b": {NodeID: "b", NewPublicKey: testKey(8)}}

	m := NewMultiSink("a", local, remote)

	got, err := m.RotateKey(context.Background(), "a")
	if err != nil {
		t.Fatalf("RotateKey(a): %v", err)
	}
	if got.NewPublicKey != testKey(9) {
		t.Fatalf("RotateKey(a) = %+v, want local sink's answer", got)
	}

	got, err = m.RotateKey(context.Background(), "b")
	if err != nil {
		t.Fatalf("RotateKey(b): %v", err)
	}
	if got.NewPublicKey != testKey(8) {
		t.Fatalf("RotateKey(b) = %+v, want remote sink's answer", got)
	}
}

func TestMultiSink_RotateKey_NilSink_FailsClearly(t *testing.T) {
	local := newRecordingSink()
	m := NewMultiSink("a", local, nil)

	if _, err := m.RotateKey(context.Background(), "b"); err == nil {
		t.Fatal("RotateKey() error = nil, want an error for an unconfigured remote sink")
	}
}

func TestMultiSink_RotateKey_SinkWithoutKeyRotator_ReturnsErrRotationNotSupported(t *testing.T) {
	local := readOnlySink{recordingSink: newRecordingSink()}
	m := NewMultiSink("a", local, nil)

	_, err := m.RotateKey(context.Background(), "a")
	if !errors.Is(err, ErrRotationNotSupported) {
		t.Fatalf("RotateKey() error = %v, want ErrRotationNotSupported", err)
	}
}

// recordingRotatorSink layers KeyRotator onto recordingSink for tests
// that need both ConfigSink and KeyRotator on the same fake.
type recordingRotatorSink struct {
	*recordingSink
	rotations map[string]RotationResult
	failures  map[string]error
}

func (s *recordingRotatorSink) RotateKey(_ context.Context, nodeID string) (RotationResult, error) {
	if err, bad := s.failures[nodeID]; bad {
		return RotationResult{}, err
	}
	return s.rotations[nodeID], nil
}
