package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// fakeSessionStream is a hand-written fake for sessionStream, the same
// "not a mocking framework" convention every other package's tests in
// this codebase already follow.
type fakeSessionStream struct {
	sent    chan *agentpb.ControlMessage
	recv    chan *agentpb.AgentMessage
	recvErr error
	sendErr error
}

func newFakeSessionStream() *fakeSessionStream {
	return &fakeSessionStream{
		sent: make(chan *agentpb.ControlMessage, 8),
		recv: make(chan *agentpb.AgentMessage, 8),
	}
}

func (f *fakeSessionStream) Send(msg *agentpb.ControlMessage) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent <- msg
	return nil
}

func (f *fakeSessionStream) Recv() (*agentpb.AgentMessage, error) {
	msg, ok := <-f.recv
	if !ok {
		return nil, f.recvErr
	}
	return msg, nil
}

func waitSentRequestID(t *testing.T, stream *fakeSessionStream) string {
	t.Helper()
	select {
	case msg := <-stream.sent:
		return msg.GetRequest().GetRequestId()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the mux to send a request")
		return ""
	}
}

func TestMux_Call_Success(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)

	done := make(chan struct {
		resp *agentpb.AgentResponse
		err  error
	}, 1)
	go func() {
		resp, err := m.Call(context.Background(), &agentpb.AgentRequest{
			Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
		})
		done <- struct {
			resp *agentpb.AgentResponse
			err  error
		}{resp, err}
	}()

	id := waitSentRequestID(t, stream)
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
		Response: &agentpb.AgentResponse{RequestId: id, Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}},
	}}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Call() error = %v", got.err)
		}
		if got.resp.GetRequestId() != id {
			t.Errorf("resp.RequestId = %q, want %q", got.resp.GetRequestId(), id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Call() to return")
	}
}

func TestMux_Call_ErrorResponse_BecomesGoError(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)

	done := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), &agentpb.AgentRequest{
			Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}},
		})
		done <- err
	}()

	id := waitSentRequestID(t, stream)
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
		Response: &agentpb.AgentResponse{RequestId: id, Error: "container not found"},
	}}

	select {
	case err := <-done:
		if err == nil || err.Error() != "container not found" {
			t.Errorf("Call() error = %v, want %q", err, "container not found")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestMux_Call_ContextCancelled(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := m.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}})
		done <- err
	}()

	waitSentRequestID(t, stream) // ensure the request was actually sent before cancelling
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestMux_StreamCloses_FailsPendingCall(t *testing.T) {
	stream := newFakeSessionStream()
	stream.recvErr = errors.New("connection reset")
	m := newMux(stream)

	done := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}})
		done <- err
	}()

	waitSentRequestID(t, stream)
	close(stream.recv) // triggers Recv() to return recvErr, ending recvLoop

	select {
	case err := <-done:
		if !errors.Is(err, ErrSessionClosed) {
			t.Errorf("Call() error = %v, want ErrSessionClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestMux_Call_AfterStreamClosed_FailsImmediately(t *testing.T) {
	stream := newFakeSessionStream()
	stream.recvErr = errors.New("connection reset")
	m := newMux(stream)

	close(stream.recv)
	<-m.closed // wait for shutdown to actually complete before calling

	_, err := m.Call(context.Background(), &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}})
	if !errors.Is(err, ErrSessionClosed) {
		t.Errorf("Call() error = %v, want ErrSessionClosed", err)
	}
}

func TestMux_Call_SendError(t *testing.T) {
	stream := newFakeSessionStream()
	stream.sendErr = errors.New("write: broken pipe")
	m := newMux(stream)

	_, err := m.Call(context.Background(), &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}})
	if err == nil {
		t.Fatal("Call() error = nil, want the send error to propagate")
	}
}

func TestMux_Event_DeliveredToSubscriber(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)
	got := m.subscribe("w1")

	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{
		Event: &agentpb.ProxiedEvent{WatchId: "w1", Action: "start", ContainerName: "web-1"},
	}}

	select {
	case ev := <-got:
		if ev.WatchId != "w1" || ev.ContainerName != "web-1" {
			t.Errorf("got = %+v, want WatchId=w1 ContainerName=web-1", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the event")
	}
}

func TestMux_Event_UnknownWatchID_Dropped(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)
	got := m.subscribe("w1")

	// An event for a watch_id nobody subscribed to must not be
	// delivered to an unrelated subscriber, and must not panic.
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{
		Event: &agentpb.ProxiedEvent{WatchId: "someone-elses-watch"},
	}}
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{
		Event: &agentpb.ProxiedEvent{WatchId: "w1", ContainerName: "web-1"},
	}}

	select {
	case ev := <-got:
		if ev.ContainerName != "web-1" {
			t.Errorf("got = %+v, want the w1 event, not the stray one", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestMux_Unsubscribe_ClosesChannel(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)
	got := m.subscribe("w1")

	m.unsubscribe("w1")

	select {
	case _, ok := <-got:
		if ok {
			t.Error("channel yielded a value instead of being closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the channel to close")
	}
}

func TestMux_StreamCloses_ClosesEventSubscriptions(t *testing.T) {
	stream := newFakeSessionStream()
	stream.recvErr = errors.New("connection reset")
	m := newMux(stream)
	got := m.subscribe("w1")

	close(stream.recv)

	select {
	case _, ok := <-got:
		if ok {
			t.Error("channel yielded a value instead of being closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the channel to close when the stream ends")
	}
}

func TestMux_Subscribe_AfterClosed_ReturnsNil(t *testing.T) {
	stream := newFakeSessionStream()
	stream.recvErr = errors.New("connection reset")
	m := newMux(stream)

	close(stream.recv)
	<-m.closed

	if ch := m.subscribe("w1"); ch != nil {
		t.Error("subscribe() after the mux closed returned a non-nil channel, want nil")
	}
}

func TestMux_UnknownResponseID_Dropped(t *testing.T) {
	stream := newFakeSessionStream()
	m := newMux(stream)

	// A response for a request_id nobody is waiting on must not panic
	// or otherwise misbehave.
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
		Response: &agentpb.AgentResponse{RequestId: "nobody-waiting"},
	}}

	// The mux must still work normally afterward.
	done := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}})
		done <- err
	}()
	id := waitSentRequestID(t, stream)
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
		Response: &agentpb.AgentResponse{RequestId: id, Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}},
	}}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Call() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}
