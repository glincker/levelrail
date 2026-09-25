package agent

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/network"
)

// fakeAgentClientStream is a hand-written fake for agentClientStream.
type fakeAgentClientStream struct {
	recv    chan *agentpb.ControlMessage
	sent    chan *agentpb.AgentMessage
	recvErr error
}

func newFakeAgentClientStream() *fakeAgentClientStream {
	return &fakeAgentClientStream{
		recv: make(chan *agentpb.ControlMessage, 8),
		sent: make(chan *agentpb.AgentMessage, 8),
	}
}

func (f *fakeAgentClientStream) Send(msg *agentpb.AgentMessage) error {
	f.sent <- msg
	return nil
}

func (f *fakeAgentClientStream) Recv() (*agentpb.ControlMessage, error) {
	msg, ok := <-f.recv
	if !ok {
		return nil, f.recvErr
	}
	return msg, nil
}

func controlRequest(req *agentpb.AgentRequest) *agentpb.ControlMessage {
	return &agentpb.ControlMessage{Payload: &agentpb.ControlMessage_Request{Request: req}}
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestServeSession_DispatchesRequest_SendsResponse(t *testing.T) {
	stream := newFakeAgentClientStream()
	rt := newExecRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, stream, rt, nil, nil, "", time.Hour, nil, testLogger()) }()

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
	})

	select {
	case msg := <-stream.sent:
		resp := msg.GetResponse()
		if resp == nil {
			t.Fatalf("sent message = %+v, want a Response payload", msg)
		}
		if resp.GetRequestId() != "r1" {
			t.Errorf("RequestId = %q, want r1", resp.GetRequestId())
		}
		if resp.GetEmpty() == nil {
			t.Errorf("resp = %+v, want an Empty result for Start", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a response")
	}
	if rt.startID != "c1" {
		t.Errorf("startID = %q, want c1", rt.startID)
	}

	stream.recvErr = errors.New("connection reset")
	close(stream.recv)
	select {
	case err := <-done:
		if err == nil {
			t.Error("serveSession() error = nil, want an error once Recv fails")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for serveSession() to return")
	}
}

func TestServeSession_RecvError_ReturnsImmediately(t *testing.T) {
	stream := newFakeAgentClientStream()
	stream.recvErr = errors.New("connection reset")
	close(stream.recv)

	err := serveSession(context.Background(), stream, newExecRuntime(), nil, nil, "", time.Hour, nil, testLogger())
	if err == nil {
		t.Fatal("serveSession() error = nil, want the recv error wrapped")
	}
}

func TestServeSession_MultipleRequests_AllAnswered(t *testing.T) {
	stream := newFakeAgentClientStream()
	rt := newExecRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = serveSession(ctx, stream, rt, nil, nil, "", time.Hour, nil, testLogger()) }()

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r1", Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
	})
	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r2", Op: &agentpb.AgentRequest_EnsureVolume{EnsureVolume: &agentpb.EnsureVolumeRequest{Name: "v1"}},
	})

	seen := map[string]bool{}
	for range 2 {
		select {
		case msg := <-stream.sent:
			seen[msg.GetResponse().GetRequestId()] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for both responses")
		}
	}
	if !seen["r1"] || !seen["r2"] {
		t.Errorf("seen = %v, want both r1 and r2 answered", seen)
	}
}

func TestServeSession_WatchEvents_EmitsProxiedEvent(t *testing.T) {
	stream := newFakeAgentClientStream()
	rt := newExecRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = serveSession(ctx, stream, rt, nil, nil, "", time.Hour, nil, testLogger()) }()

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_WatchEvents{WatchEvents: &agentpb.WatchEventsRequest{WatchId: "w1"}},
	})

	// First frame back is the WatchEvents acknowledgment.
	select {
	case msg := <-stream.sent:
		if msg.GetResponse().GetRequestId() != "r1" {
			t.Fatalf("first sent message = %+v, want the WatchEvents ack", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the WatchEvents ack")
	}

	rt.events <- docker.Event{Action: docker.EventStart, ContainerName: "web-1", Time: time.Now()}

	select {
	case msg := <-stream.sent:
		ev := msg.GetEvent()
		if ev == nil || ev.WatchId != "w1" || ev.ContainerName != "web-1" {
			t.Errorf("sent message = %+v, want a ProxiedEvent for w1/web-1", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the relayed event")
	}
}

// TestServeSession_DispatchesMeshRequests exercises the same path a real
// control plane connection uses (WithMesh -> serveSession's own
// ApplyMesh/RotateMeshKey branches, mesh_dispatch.go), rather than
// calling handleApplyMesh/handleRotateMeshKey directly the way
// mesh_dispatch_test.go does: this is the end-to-end proof that a
// mesh-enabled Session actually reaches the applier, not just that the
// handlers work in isolation.
func TestServeSession_DispatchesMeshRequests(t *testing.T) {
	stream := newFakeAgentClientStream()
	applier := &fakeMeshApplier{
		applyResult:  network.NodeIdentity{PublicKey: testMeshKey(t, 6)},
		rotateResult: network.RotationResult{OldPublicKey: testMeshKey(t, 1), NewPublicKey: testMeshKey(t, 2)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = serveSession(ctx, stream, newExecRuntime(), nil, applier, "node-a", time.Hour, nil, testLogger())
	}()

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_ApplyMesh{ApplyMesh: &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{NodeId: "node-a"}}},
	})
	select {
	case msg := <-stream.sent:
		resp := msg.GetResponse()
		if resp.GetError() != "" {
			t.Fatalf("ApplyMesh response error = %q, want none", resp.GetError())
		}
		if resp.GetApplyMesh().GetIdentity().GetPublicKey() != applier.applyResult.PublicKey.String() {
			t.Errorf("ApplyMesh response PublicKey = %q, want %q", resp.GetApplyMesh().GetIdentity().GetPublicKey(), applier.applyResult.PublicKey.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the ApplyMesh response")
	}
	if applier.appliedNodeID != "node-a" {
		t.Errorf("ApplyMesh dispatched with nodeID = %q, want %q", applier.appliedNodeID, "node-a")
	}

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r2",
		Op:        &agentpb.AgentRequest_RotateMeshKey{RotateMeshKey: &agentpb.RotateMeshKeyRequest{}},
	})
	select {
	case msg := <-stream.sent:
		resp := msg.GetResponse()
		if resp.GetError() != "" {
			t.Fatalf("RotateMeshKey response error = %q, want none", resp.GetError())
		}
		if resp.GetRotateMeshKey().GetNewPublicKey() != applier.rotateResult.NewPublicKey.String() {
			t.Errorf("RotateMeshKey response NewPublicKey = %q, want %q", resp.GetRotateMeshKey().GetNewPublicKey(), applier.rotateResult.NewPublicKey.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the RotateMeshKey response")
	}
	if applier.rotatedNodeID != "node-a" {
		t.Errorf("RotateMeshKey dispatched with nodeID = %q, want %q", applier.rotatedNodeID, "node-a")
	}
}

// TestServeSession_MeshRequests_NoMeshApplier_ReturnsErrMeshUnavailable
// confirms a Session started with no WithMesh option (an agent whose
// node has mesh networking disabled, or predates mesh support) answers
// clearly rather than hanging or panicking.
func TestServeSession_MeshRequests_NoMeshApplier_ReturnsErrMeshUnavailable(t *testing.T) {
	stream := newFakeAgentClientStream()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = serveSession(ctx, stream, newExecRuntime(), nil, nil, "", time.Hour, nil, testLogger()) }()

	stream.recv <- controlRequest(&agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_ApplyMesh{ApplyMesh: &agentpb.ApplyMeshRequest{Config: &agentpb.DeviceConfig{}}},
	})
	select {
	case msg := <-stream.sent:
		if msg.GetResponse().GetError() != ErrMeshUnavailable.Error() {
			t.Errorf("ApplyMesh response error = %q, want %q", msg.GetResponse().GetError(), ErrMeshUnavailable.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the ApplyMesh response")
	}
}
