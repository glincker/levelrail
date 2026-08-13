package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// respondToNextCall reads the next ControlMessage the transport sent,
// builds an AgentResponse from build (which receives the request so a
// test can inspect what was actually sent), and feeds it back through
// the fake stream's recv side, tagged with the right RequestId.
func respondToNextCall(t *testing.T, stream *fakeSessionStream, build func(req *agentpb.AgentRequest) *agentpb.AgentResponse) *agentpb.AgentRequest {
	t.Helper()
	select {
	case msg := <-stream.sent:
		req := msg.GetRequest()
		resp := build(req)
		resp.RequestId = req.GetRequestId()
		stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}}
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a request")
		return nil
	}
}

func TestGRPCTransport_InspectByName_Found(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan struct {
		state *docker.ContainerState
		err   error
	}, 1)
	go func() {
		state, err := tr.InspectByName(context.Background(), "web-1")
		done <- struct {
			state *docker.ContainerState
			err   error
		}{state, err}
	}()

	req := respondToNextCall(t, stream, func(req *agentpb.AgentRequest) *agentpb.AgentResponse {
		if req.GetInspectByName().GetName() != "web-1" {
			t.Errorf("request name = %q, want web-1", req.GetInspectByName().GetName())
		}
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_InspectByName{InspectByName: &agentpb.InspectByNameResponse{
			Found: true,
			State: &agentpb.ContainerState{Id: "abc", Name: "web-1", Running: true},
		}}}
	})
	_ = req

	got := <-done
	if got.err != nil {
		t.Fatalf("InspectByName() error = %v", got.err)
	}
	if got.state == nil || got.state.ID != "abc" || !got.state.Running {
		t.Errorf("state = %+v, want ID=abc Running=true", got.state)
	}
}

func TestGRPCTransport_InspectByName_NotFound(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan *docker.ContainerState, 1)
	go func() {
		state, err := tr.InspectByName(context.Background(), "ghost")
		if err != nil {
			t.Errorf("InspectByName() error = %v", err)
		}
		done <- state
	}()

	respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_InspectByName{InspectByName: &agentpb.InspectByNameResponse{Found: false}}}
	})

	if got := <-done; got != nil {
		t.Errorf("state = %+v, want nil (not found, not an error)", got)
	}
}

func TestGRPCTransport_Create(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan string, 1)
	go func() {
		id, err := tr.Create(context.Background(), docker.ContainerSpec{Name: "web", Image: "img:v1"})
		if err != nil {
			t.Errorf("Create() error = %v", err)
		}
		done <- id
	}()

	respondToNextCall(t, stream, func(req *agentpb.AgentRequest) *agentpb.AgentResponse {
		if req.GetCreate().GetSpec().GetImage() != "img:v1" {
			t.Errorf("spec.Image = %q, want img:v1", req.GetCreate().GetSpec().GetImage())
		}
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_Create{Create: &agentpb.CreateResponse{Id: "container-1"}}}
	})

	if got := <-done; got != "container-1" {
		t.Errorf("id = %q, want container-1", got)
	}
}

func TestGRPCTransport_Stop_NegativeTimeoutCarriesThrough(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan error, 1)
	go func() {
		done <- tr.Stop(context.Background(), "c1", -1)
	}()

	req := respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}}
	})
	if req.GetStop().GetTimeoutMs() != -1 {
		t.Errorf("TimeoutMs = %d, want -1", req.GetStop().GetTimeoutMs())
	}
	if err := <-done; err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestGRPCTransport_ListImages(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan []docker.ImageInfo, 1)
	go func() {
		images, err := tr.ListImages(context.Background(), "levelrail/web")
		if err != nil {
			t.Errorf("ListImages() error = %v", err)
		}
		done <- images
	}()

	respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_ListImages{ListImages: &agentpb.ListImagesResponse{
			Images: []*agentpb.ImageInfo{{Tag: "levelrail/web:v1"}},
		}}}
	})

	got := <-done
	if len(got) != 1 || got[0].Tag != "levelrail/web:v1" {
		t.Errorf("got = %+v", got)
	}
}

func TestGRPCTransport_RemoteError_Propagates(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))

	done := make(chan error, 1)
	go func() {
		_, err := tr.Create(context.Background(), docker.ContainerSpec{Name: "web"})
		done <- err
	}()

	respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Error: "image not found locally"}
	})

	err := <-done
	if err == nil || err.Error() != "image not found locally" {
		t.Errorf("err = %v, want %q", err, "image not found locally")
	}
}

func TestGRPCTransport_Events_RelaysUntilContextCancelled(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	ctx, cancel := context.WithCancel(context.Background())

	events, errs := tr.Events(ctx)

	// The transport must have sent a WatchEvents request to get here.
	watchReq := respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}}
	})
	watchID := watchReq.GetWatchEvents().GetWatchId()
	if watchID == "" {
		t.Fatal("WatchEvents request has no watch_id")
	}

	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{
		Event: &agentpb.ProxiedEvent{WatchId: watchID, Action: "start", ContainerName: "web-1"},
	}}

	select {
	case ev := <-events:
		if ev.Action != docker.EventStart || ev.ContainerName != "web-1" {
			t.Errorf("event = %+v, want Action=start ContainerName=web-1", ev)
		}
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the relayed event")
	}

	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Error("events channel yielded a value after cancellation instead of closing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for events channel to close after cancel")
	}
}

func TestGRPCTransport_Events_SessionClosed_ReturnsError(t *testing.T) {
	stream := newFakeSessionStream()
	stream.recvErr = errors.New("connection reset")
	tr := newGRPCTransport(newMux(stream))

	close(stream.recv)
	// Give recvLoop a moment to actually process the close and shut the
	// mux down before Events() races it.
	time.Sleep(50 * time.Millisecond)

	events, errs := tr.Events(context.Background())
	select {
	case err := <-errs:
		if !errors.Is(err, ErrSessionClosed) {
			t.Errorf("err = %v, want ErrSessionClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
	if _, ok := <-events; ok {
		t.Error("events channel yielded a value, want it closed")
	}
}
