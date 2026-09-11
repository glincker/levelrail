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

func TestGRPCTransport_Networks(t *testing.T) {
	tests := []struct {
		name string
		// call drives one transport method and reports what came back,
		// flattened so every case can share one result shape.
		call     func(*GRPCTransport) (string, []docker.NetworkInfo, error)
		respond  func(*agentpb.AgentRequest) *agentpb.AgentResponse
		wantErr  string
		wantID   string
		wantNets []docker.NetworkInfo
	}{
		{
			name: "ensure network",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				id, err := tr.EnsureNetwork(context.Background(), "levelrail-app-web")
				return id, nil, err
			},
			respond: func(req *agentpb.AgentRequest) *agentpb.AgentResponse {
				if got := req.GetEnsureNetwork().GetName(); got != "levelrail-app-web" {
					return &agentpb.AgentResponse{Error: "unexpected name " + got}
				}
				return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_EnsureNetwork{
					EnsureNetwork: &agentpb.EnsureNetworkResponse{Id: "net-abc"},
				}}
			},
			wantID: "net-abc",
		},
		{
			name: "ensure network remote error",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				id, err := tr.EnsureNetwork(context.Background(), "levelrail-app-web")
				return id, nil, err
			},
			respond: func(*agentpb.AgentRequest) *agentpb.AgentResponse {
				return &agentpb.AgentResponse{Error: "network create refused"}
			},
			wantErr: "network create refused",
		},
		{
			name: "remove network",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				return "", nil, tr.RemoveNetwork(context.Background(), "levelrail-app-old")
			},
			respond: func(req *agentpb.AgentRequest) *agentpb.AgentResponse {
				if got := req.GetRemoveNetwork().GetName(); got != "levelrail-app-old" {
					return &agentpb.AgentResponse{Error: "unexpected name " + got}
				}
				return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}}
			},
		},
		{
			name: "remove network remote error",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				return "", nil, tr.RemoveNetwork(context.Background(), "levelrail-app-old")
			},
			respond: func(*agentpb.AgentRequest) *agentpb.AgentResponse {
				return &agentpb.AgentResponse{Error: "network still in use"}
			},
			wantErr: "network still in use",
		},
		{
			name: "list networks by prefix",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				nets, err := tr.ListNetworksByPrefix(context.Background(), "levelrail-app-")
				return "", nets, err
			},
			respond: func(req *agentpb.AgentRequest) *agentpb.AgentResponse {
				if got := req.GetListNetworksByPrefix().GetPrefix(); got != "levelrail-app-" {
					return &agentpb.AgentResponse{Error: "unexpected prefix " + got}
				}
				return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_ListNetworksByPrefix{
					ListNetworksByPrefix: &agentpb.ListNetworksByPrefixResponse{
						Networks: []*agentpb.NetworkInfo{{Id: "n1", Name: "levelrail-app-a"}},
					},
				}}
			},
			wantNets: []docker.NetworkInfo{{ID: "n1", Name: "levelrail-app-a"}},
		},
		{
			name: "list networks by prefix remote error",
			call: func(tr *GRPCTransport) (string, []docker.NetworkInfo, error) {
				nets, err := tr.ListNetworksByPrefix(context.Background(), "levelrail-app-")
				return "", nets, err
			},
			respond: func(*agentpb.AgentRequest) *agentpb.AgentResponse {
				return &agentpb.AgentResponse{Error: "daemon unreachable"}
			},
			wantErr: "daemon unreachable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := newFakeSessionStream()
			tr := newGRPCTransport(newMux(stream))

			type result struct {
				id   string
				nets []docker.NetworkInfo
				err  error
			}
			done := make(chan result, 1)
			go func() {
				id, nets, err := tc.call(tr)
				done <- result{id, nets, err}
			}()

			respondToNextCall(t, stream, tc.respond)

			got := <-done
			if tc.wantErr != "" {
				if got.err == nil || got.err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", got.err, tc.wantErr)
				}
				return
			}
			if got.err != nil {
				t.Fatalf("unexpected error: %v", got.err)
			}
			if got.id != tc.wantID {
				t.Errorf("id = %q, want %q", got.id, tc.wantID)
			}
			if len(got.nets) != len(tc.wantNets) {
				t.Fatalf("networks = %+v, want %+v", got.nets, tc.wantNets)
			}
			for i := range tc.wantNets {
				if got.nets[i] != tc.wantNets[i] {
					t.Errorf("networks[%d] = %+v, want %+v", i, got.nets[i], tc.wantNets[i])
				}
			}
		})
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
