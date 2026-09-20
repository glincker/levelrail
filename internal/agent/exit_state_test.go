package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// serveFakeAgent answers every request the transport sends by running
// Execute against rt, the same dispatch serveSession does for a real
// agent, so these tests exercise the actual wire encoding in both
// directions rather than a stubbed response.
func serveFakeAgent(t *testing.T, stream *fakeSessionStream, rt docker.Runtime) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-stream.sent:
				req := msg.GetRequest()
				if req == nil {
					continue
				}
				go func() {
					resp := Execute(ctx, rt, req, func(*agentpb.ProxiedEvent) {})
					select {
					case stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}}:
					case <-ctx.Done():
					}
				}()
			}
		}
	}()
}

// exitStateRuntime is transport_test.go's minimal fakeRuntime plus the
// optional docker.ExitStateInspector capability, standing in for the
// *docker.Client a real agent holds on its own node.
type exitStateRuntime struct {
	fakeRuntime
	state    *docker.ExitState
	lastName string
}

func (f *exitStateRuntime) InspectExitState(_ context.Context, name string) (*docker.ExitState, error) {
	f.lastName = name
	return f.state, nil
}

func TestGRPCTransport_InspectExitState_OOMKilled(t *testing.T) {
	rt := &exitStateRuntime{state: &docker.ExitState{Running: false, OOMKilled: true, ExitCode: 137}}
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, rt)

	// The type assertion application.Controller.waitReady itself makes,
	// on the same static type a remote node's controller is handed.
	var runtime docker.Runtime = tr
	inspector, ok := runtime.(docker.ExitStateInspector)
	if !ok {
		t.Fatal("GRPCTransport is not a docker.ExitStateInspector: a remote node would silently skip the fast-fail-on-crash path")
	}

	got, err := inspector.InspectExitState(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("InspectExitState() error = %v", err)
	}
	if got == nil || got.Running || !got.OOMKilled || got.ExitCode != 137 {
		t.Errorf("state = %+v, want Running=false OOMKilled=true ExitCode=137", got)
	}
	if rt.lastName != "web-1" {
		t.Errorf("agent saw name = %q, want web-1", rt.lastName)
	}
}

func TestGRPCTransport_InspectExitState_PlainExit(t *testing.T) {
	rt := &exitStateRuntime{state: &docker.ExitState{Running: false, OOMKilled: false, ExitCode: 1}}
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, rt)

	got, err := tr.InspectExitState(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("InspectExitState() error = %v", err)
	}
	if got == nil || got.OOMKilled || got.ExitCode != 1 {
		t.Errorf("state = %+v, want OOMKilled=false ExitCode=1", got)
	}
}

func TestGRPCTransport_InspectExitState_StillRunning(t *testing.T) {
	rt := &exitStateRuntime{state: &docker.ExitState{Running: true}}
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, rt)

	got, err := tr.InspectExitState(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("InspectExitState() error = %v", err)
	}
	if got == nil || !got.Running {
		t.Errorf("state = %+v, want Running=true", got)
	}
}

func TestGRPCTransport_InspectExitState_NotFound(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, &exitStateRuntime{state: nil})

	got, err := tr.InspectExitState(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("InspectExitState() error = %v", err)
	}
	if got != nil {
		t.Errorf("state = %+v, want nil (not found, not an error)", got)
	}
}

// TestGRPCTransport_InspectExitState_RuntimeWithoutCapability proves the
// degraded path stays degraded rather than becoming a false crash
// report: an agent whose runtime cannot inspect exit state answers with
// an error, which every caller treats as "can't tell."
func TestGRPCTransport_InspectExitState_RuntimeWithoutCapability(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, &fakeRuntime{})

	got, err := tr.InspectExitState(context.Background(), "web-1")
	if err == nil {
		t.Fatal("InspectExitState() error = nil, want the unsupported-capability error")
	}
	if !strings.Contains(err.Error(), "cannot inspect container exit state") {
		t.Errorf("error = %v, want it to carry ErrExitStateUnsupported's message", err)
	}
	if got != nil {
		t.Errorf("state = %+v, want nil alongside the error", got)
	}
}

func TestLocal_InspectExitState_DelegatesToWrappedRuntime(t *testing.T) {
	rt := &exitStateRuntime{state: &docker.ExitState{Running: false, OOMKilled: true, ExitCode: 137}}
	local := NewLocal(rt)

	got, err := local.InspectExitState(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("InspectExitState() error = %v", err)
	}
	if got == nil || !got.OOMKilled {
		t.Errorf("state = %+v, want the wrapped runtime's own state returned unchanged", got)
	}
}

func TestLocal_InspectExitState_RuntimeWithoutCapability(t *testing.T) {
	local := NewLocal(&fakeRuntime{})

	if _, err := local.InspectExitState(context.Background(), "web-1"); err != ErrExitStateUnsupported { //nolint:errorlint // the sentinel is returned directly, not wrapped
		t.Errorf("error = %v, want ErrExitStateUnsupported", err)
	}
}

// wireRuntime is a stateful fake docker.Runtime, enough of a container
// lifecycle for a real application.Controller to deploy against it: the
// remote-node counterpart of that package's own test fake, reached here
// only through a GRPCTransport.
type wireRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	exitStates map[string]*docker.ExitState
	networks   map[string]string
	nextID     int
	hostPort   int
}

func newWireRuntime(hostPort int) *wireRuntime {
	return &wireRuntime{
		containers: map[string]*docker.ContainerState{},
		exitStates: map[string]*docker.ExitState{},
		networks:   map[string]string{},
		hostPort:   hostPort,
	}
}

// crash records name as exited without clearing its Running flag: the
// divergence that makes this failure mode dangerous, since Docker's
// container list still reports a container a real inspect already knows
// is dead.
func (f *wireRuntime) crash(name string, oomKilled bool, exitCode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exitStates[name] = &docker.ExitState{Running: false, OOMKilled: oomKilled, ExitCode: exitCode}
}

func (f *wireRuntime) InspectExitState(_ context.Context, name string) (*docker.ExitState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	es, ok := f.exitStates[name]
	if !ok {
		return nil, nil
	}
	cp := *es
	return &cp, nil
}

func (f *wireRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *wireRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := strconv.Itoa(f.nextID)
	f.containers[spec.Name] = &docker.ContainerState{ID: id, Name: spec.Name, Image: spec.Image}
	return id, nil
}

func (f *wireRuntime) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = true
			if f.hostPort != 0 {
				cs.Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: f.hostPort}}
			}
		}
	}
	return nil
}

func (f *wireRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = false
		}
	}
	return nil
}

func (f *wireRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, cs := range f.containers {
		if cs.ID == id {
			delete(f.containers, name)
		}
	}
	return nil
}

func (f *wireRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.ContainerState
	for name, cs := range f.containers {
		if strings.HasPrefix(name, prefix) {
			out = append(out, *cs)
		}
	}
	return out, nil
}

func (f *wireRuntime) EnsureNetwork(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.networks[name]; ok {
		return id, nil
	}
	f.nextID++
	id := strconv.Itoa(f.nextID)
	f.networks[name] = id
	return id, nil
}

func (f *wireRuntime) RemoveNetwork(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.networks, name)
	return nil
}

func (f *wireRuntime) ListNetworksByPrefix(_ context.Context, prefix string) ([]docker.NetworkInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.NetworkInfo
	for name, id := range f.networks {
		if strings.HasPrefix(name, prefix) {
			out = append(out, docker.NetworkInfo{ID: id, Name: name})
		}
	}
	return out, nil
}

func (f *wireRuntime) UpdateResources(context.Context, string, docker.Resources) error { return nil }
func (f *wireRuntime) EnsureVolume(context.Context, string) error                      { return nil }
func (f *wireRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *wireRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) { return nil, nil }
func (f *wireRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return nil, errors.New("wireRuntime: Exec not implemented")
}
func (f *wireRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("wireRuntime: ExecWithInput not implemented")
}

// wireStore is the one desired-state lookup application.Controller makes.
type wireStore struct {
	svc *store.DesiredService
}

func (s *wireStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return s.svc, nil
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parsing test server URL %q: %v", srv.URL, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parsing test server port %q: %v", port, err)
	}
	return n
}

// TestController_RemoteNode_FastFailsOnOOMKillDuringReadiness is the
// whole point of the wire method above: a deploy onto a remote node now
// surfaces an OOM kill as soon as it happens, instead of waiting out the
// full readiness budget and reporting a generic timeout, matching what a
// locally placed service already did.
func TestController_RemoteNode_FastFailsOnOOMKillDuringReadiness(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	rt := newWireRuntime(serverPort(t, srv))
	stream := newFakeSessionStream()
	transport := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, rt)

	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	target := application.ContainerName("web", desired.Image, "")
	time.AfterFunc(30*time.Millisecond, func() { rt.crash(target, true, 137) })

	c := application.New("web", &wireStore{svc: desired}, transport, application.WithReadyBudget(5*time.Second))

	start := time.Now()
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the OOM kill surfaced")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Reconcile took %v, want a fast fail well inside the 5s readiness budget", elapsed)
	}
	if len(result.Conditions) == 0 {
		t.Fatal("expected at least one condition, got none")
	}
	cond := result.Conditions[0]
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "OOMKilledDuringReadiness" {
		t.Errorf("condition = %+v, want Status=False Reason=OOMKilledDuringReadiness", cond)
	}
}
