package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

func startReq(id, containerID string) *agentpb.ControlMessage {
	return controlRequest(&agentpb.AgentRequest{
		RequestId: id,
		Op:        &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: containerID}},
	})
}

func awaitResponse(t *testing.T, stream *fakeAgentClientStream, id string) {
	t.Helper()
	select {
	case msg := <-stream.sent:
		if resp := msg.GetResponse(); resp == nil || resp.GetRequestId() != id || resp.GetError() != "" {
			t.Fatalf("sent message = %+v, want a clean response for %s", msg, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for response %s", id)
	}
}

// A severed stream must end the session with an error and touch no
// container; the next session on the same runtime serves normally.
func TestChaos_AgentConnectionSeveredMidStream_ReconnectsWithoutTouchingContainers(t *testing.T) {
	rt := newExecRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := newFakeAgentClientStream()
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, first, rt, nil, nil, "", time.Hour, nil, testLogger()) }()
	first.recv <- startReq("r1", "c1")
	awaitResponse(t, first, "r1")

	first.recvErr = errors.New("rpc error: code = Unavailable desc = transport is closing")
	close(first.recv)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serveSession() error = nil, want the severed stream surfaced")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveSession() did not return after the stream broke")
	}
	if rt.stopID != "" || rt.removeID != "" {
		t.Fatalf("severed connection touched containers: stop=%q remove=%q", rt.stopID, rt.removeID)
	}

	second := newFakeAgentClientStream()
	done2 := make(chan error, 1)
	go func() { done2 <- serveSession(ctx, second, rt, nil, nil, "", time.Hour, nil, testLogger()) }()
	second.recv <- startReq("r2", "c2")
	awaitResponse(t, second, "r2")
	if rt.startID != "c2" {
		t.Errorf("startID = %q, want c2 served on the new session", rt.startID)
	}
	cancel()
}
