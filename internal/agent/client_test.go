package agent

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
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

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestServeSession_DispatchesRequest_SendsResponse(t *testing.T) {
	stream := newFakeAgentClientStream()
	rt := newExecRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveSession(ctx, stream, rt, testLogger()) }()

	stream.recv <- &agentpb.ControlMessage{Request: &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
	}}

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

	err := serveSession(context.Background(), stream, newExecRuntime(), testLogger())
	if err == nil {
		t.Fatal("serveSession() error = nil, want the recv error wrapped")
	}
}

func TestServeSession_MultipleRequests_AllAnswered(t *testing.T) {
	stream := newFakeAgentClientStream()
	rt := newExecRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = serveSession(ctx, stream, rt, testLogger()) }()

	stream.recv <- &agentpb.ControlMessage{Request: &agentpb.AgentRequest{
		RequestId: "r1", Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
	}}
	stream.recv <- &agentpb.ControlMessage{Request: &agentpb.AgentRequest{
		RequestId: "r2", Op: &agentpb.AgentRequest_EnsureVolume{EnsureVolume: &agentpb.EnsureVolumeRequest{Name: "v1"}},
	}}

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
	go func() { _ = serveSession(ctx, stream, rt, testLogger()) }()

	stream.recv <- &agentpb.ControlMessage{Request: &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_WatchEvents{WatchEvents: &agentpb.WatchEventsRequest{WatchId: "w1"}},
	}}

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
