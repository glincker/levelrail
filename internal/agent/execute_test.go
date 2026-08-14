package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// execRuntime is a hand-written, fully controllable fake docker.Runtime
// for exercising every Execute branch, distinct from transport_test.go's
// fakeRuntime (which only needs to prove delegation, not full
// call/return behavior per method).
type execRuntime struct {
	inspectState *docker.ContainerState
	inspectErr   error
	createID     string
	createErr    error
	createSpec   docker.ContainerSpec
	startErr     error
	startID      string
	stopErr      error
	stopID       string
	stopTimeout  time.Duration
	removeErr    error
	removeID     string
	removeForce  bool
	images       []docker.ImageInfo
	imagesErr    error
	imagesRepo   string
	containers   []docker.ContainerState
	prefixErr    error
	prefix       string
	volumeErr    error
	volumeName   string
	events       chan docker.Event
	eventErrs    chan error
}

func newExecRuntime() *execRuntime {
	return &execRuntime{events: make(chan docker.Event, 4), eventErrs: make(chan error, 1)}
}

func (f *execRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	_ = name
	return f.inspectState, f.inspectErr
}
func (f *execRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.createSpec = spec
	return f.createID, f.createErr
}
func (f *execRuntime) Start(_ context.Context, id string) error {
	f.startID = id
	return f.startErr
}
func (f *execRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return f.events, f.eventErrs
}
func (f *execRuntime) ListImages(_ context.Context, repo string) ([]docker.ImageInfo, error) {
	f.imagesRepo = repo
	return f.images, f.imagesErr
}
func (f *execRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	f.prefix = prefix
	return f.containers, f.prefixErr
}
func (f *execRuntime) Stop(_ context.Context, id string, timeout time.Duration) error {
	f.stopID, f.stopTimeout = id, timeout
	return f.stopErr
}
func (f *execRuntime) Remove(_ context.Context, id string, force bool) error {
	f.removeID, f.removeForce = id, force
	return f.removeErr
}
func (f *execRuntime) EnsureVolume(_ context.Context, name string) error {
	f.volumeName = name
	return f.volumeErr
}

func TestExecute_InspectByName_Found(t *testing.T) {
	rt := newExecRuntime()
	rt.inspectState = &docker.ContainerState{ID: "abc", Name: "web-1", Running: true}

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_InspectByName{InspectByName: &agentpb.InspectByNameRequest{Name: "web-1"}},
	}, nil)

	if resp.RequestId != "r1" {
		t.Errorf("RequestId = %q, want r1", resp.RequestId)
	}
	if resp.Error != "" {
		t.Fatalf("Error = %q, want empty", resp.Error)
	}
	got := resp.GetInspectByName()
	if got == nil || !got.Found || got.State.GetName() != "web-1" {
		t.Errorf("got = %+v, want Found=true State.Name=web-1", got)
	}
}

func TestExecute_InspectByName_NotFound(t *testing.T) {
	rt := newExecRuntime() // inspectState stays nil, no error: "not found, not an error"

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_InspectByName{InspectByName: &agentpb.InspectByNameRequest{Name: "ghost"}},
	}, nil)

	got := resp.GetInspectByName()
	if got == nil || got.Found {
		t.Errorf("got = %+v, want Found=false", got)
	}
}

func TestExecute_InspectByName_Error(t *testing.T) {
	rt := newExecRuntime()
	rt.inspectErr = errors.New("daemon unreachable")

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_InspectByName{InspectByName: &agentpb.InspectByNameRequest{Name: "web-1"}},
	}, nil)

	if resp.Error != "daemon unreachable" {
		t.Errorf("Error = %q, want %q", resp.Error, "daemon unreachable")
	}
	if resp.GetResult() != nil {
		t.Errorf("Result = %v, want nil on error", resp.GetResult())
	}
}

func TestExecute_Create(t *testing.T) {
	rt := newExecRuntime()
	rt.createID = "container-123"

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op: &agentpb.AgentRequest_Create{Create: &agentpb.CreateRequest{
			Spec: &agentpb.ContainerSpec{Name: "web", Image: "img:v1", Env: map[string]string{"K": "V"}},
		}},
	}, nil)

	if resp.Error != "" {
		t.Fatalf("Error = %q", resp.Error)
	}
	if resp.GetCreate().GetId() != "container-123" {
		t.Errorf("Create().Id = %q, want container-123", resp.GetCreate().GetId())
	}
	if rt.createSpec.Name != "web" || rt.createSpec.Image != "img:v1" || rt.createSpec.Env["K"] != "V" {
		t.Errorf("createSpec = %+v, want the request's spec translated through", rt.createSpec)
	}
}

func TestExecute_Start(t *testing.T) {
	rt := newExecRuntime()
	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "container-123"}},
	}, nil)
	if resp.Error != "" || resp.GetEmpty() == nil {
		t.Errorf("resp = %+v, want Error empty and Empty result set", resp)
	}
	if rt.startID != "container-123" {
		t.Errorf("startID = %q, want container-123", rt.startID)
	}
}

func TestExecute_Stop_PositiveTimeout(t *testing.T) {
	rt := newExecRuntime()
	Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Stop{Stop: &agentpb.StopRequest{Id: "c1", TimeoutMs: 5000}},
	}, nil)
	if rt.stopTimeout != 5*time.Second {
		t.Errorf("stopTimeout = %v, want 5s", rt.stopTimeout)
	}
}

func TestExecute_Stop_NegativeTimeoutMeansIndefinite(t *testing.T) {
	rt := newExecRuntime()
	Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Stop{Stop: &agentpb.StopRequest{Id: "c1", TimeoutMs: -1}},
	}, nil)
	if rt.stopTimeout != -1 {
		t.Errorf("stopTimeout = %v, want -1 (wait indefinitely)", rt.stopTimeout)
	}
}

func TestExecute_Remove(t *testing.T) {
	rt := newExecRuntime()
	Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_Remove{Remove: &agentpb.RemoveRequest{Id: "c1", Force: true}},
	}, nil)
	if rt.removeID != "c1" || !rt.removeForce {
		t.Errorf("removeID=%q removeForce=%v, want c1/true", rt.removeID, rt.removeForce)
	}
}

func TestExecute_ListImages(t *testing.T) {
	rt := newExecRuntime()
	rt.images = []docker.ImageInfo{{Tag: "levelrail/web:v1"}, {Tag: "levelrail/web:v2"}}

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_ListImages{ListImages: &agentpb.ListImagesRequest{Repo: "levelrail/web"}},
	}, nil)

	if rt.imagesRepo != "levelrail/web" {
		t.Errorf("imagesRepo = %q, want levelrail/web", rt.imagesRepo)
	}
	got := resp.GetListImages().GetImages()
	if len(got) != 2 || got[0].Tag != "levelrail/web:v1" {
		t.Errorf("got = %+v, want 2 images", got)
	}
}

func TestExecute_ListByPrefix(t *testing.T) {
	rt := newExecRuntime()
	rt.containers = []docker.ContainerState{{ID: "a", Name: "web-a"}, {ID: "b", Name: "web-b"}}

	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_ListByPrefix{ListByPrefix: &agentpb.ListByPrefixRequest{Prefix: "web-"}},
	}, nil)

	got := resp.GetListByPrefix().GetContainers()
	if len(got) != 2 || got[0].Name != "web-a" || got[1].Name != "web-b" {
		t.Errorf("got = %+v, want [web-a web-b]", got)
	}
}

func TestExecute_EnsureVolume(t *testing.T) {
	rt := newExecRuntime()
	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_EnsureVolume{EnsureVolume: &agentpb.EnsureVolumeRequest{Name: "db-data"}},
	}, nil)
	if rt.volumeName != "db-data" || resp.GetEmpty() == nil {
		t.Errorf("volumeName=%q resp=%+v, want db-data and an Empty result", rt.volumeName, resp)
	}
}

func TestExecute_UnknownOp_ReturnsError(t *testing.T) {
	resp := Execute(context.Background(), newExecRuntime(), &agentpb.AgentRequest{RequestId: "r1"}, nil)
	if resp.Error == "" {
		t.Error("Error is empty, want a message for a request with no op set")
	}
}

func TestExecute_WatchEvents_RelaysUntilContextCancelled(t *testing.T) {
	rt := newExecRuntime()
	ctx, cancel := context.WithCancel(context.Background())

	got := make(chan *agentpb.ProxiedEvent, 4)
	resp := Execute(ctx, rt, &agentpb.AgentRequest{
		RequestId: "r1",
		Op:        &agentpb.AgentRequest_WatchEvents{WatchEvents: &agentpb.WatchEventsRequest{WatchId: "w1"}},
	}, func(ev *agentpb.ProxiedEvent) { got <- ev })

	if resp.Error != "" || resp.GetEmpty() == nil {
		t.Fatalf("resp = %+v, want an immediate Empty acknowledgment", resp)
	}

	rt.events <- docker.Event{Action: docker.EventStart, ContainerName: "web-1", Time: time.Now()}

	select {
	case ev := <-got:
		if ev.WatchId != "w1" || ev.Action != "start" || ev.ContainerName != "web-1" {
			t.Errorf("relayed event = %+v, want WatchId=w1 Action=start ContainerName=web-1", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the relayed event")
	}

	cancel()
	// Draining the events channel after cancel must not panic or hang;
	// relayEvents' own select on ctx.Done() is what's actually under
	// test here.
	select {
	case rt.events <- docker.Event{Action: docker.EventDie, ContainerName: "web-1"}:
	default:
	}
}
