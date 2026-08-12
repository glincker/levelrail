package application

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, not a mocking framework, the same
// pattern nginxdemo's tests already established for docker.Runtime.
type fakeStore struct {
	svc *store.DesiredService
	err error
}

func (f *fakeStore) GetDesiredService(_ context.Context, _ string) (*store.DesiredService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.svc, nil
}

// fakeRuntime is a stateful fake, not just a call counter: it tracks an
// in-memory set of "containers" so multi-step scenarios (create, then
// probe, then clean up something else) can be tested realistically,
// exactly the kind of scenario this controller's logic actually has.
// Every created container's Ports point at hostPort, standing in for one
// real backend (usually an httptest.Server) every test wires up.
type fakeRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	nextID     int
	hostPort   int

	createErr error
	startErr  error
	stopErr   error
	removeErr error

	createCalls int
	removeCalls int
}

func newFakeRuntime(hostPort int) *fakeRuntime {
	return &fakeRuntime{containers: map[string]*docker.ContainerState{}, hostPort: hostPort}
}

func (f *fakeRuntime) seed(name string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	cs := &docker.ContainerState{ID: strconv.Itoa(f.nextID), Name: name, Running: running}
	if running && f.hostPort != 0 {
		cs.Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: f.hostPort}}
	}
	f.containers[name] = cs
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.createErr != nil {
		return "", f.createErr
	}
	f.nextID++
	id := strconv.Itoa(f.nextID)
	f.containers[spec.Name] = &docker.ContainerState{ID: id, Name: spec.Name, Image: spec.Image}
	return id, nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
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

func (f *fakeRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopErr != nil {
		return f.stopErr
	}
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = false
		}
	}
	return nil
}

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls++
	if f.removeErr != nil {
		return f.removeErr
	}
	for name, cs := range f.containers {
		if cs.ID == id {
			delete(f.containers, name)
		}
	}
	return nil
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
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

func (f *fakeRuntime) ListImages(_ context.Context, _ string) ([]docker.ImageInfo, error) {
	return nil, nil
}

// EnsureVolume is a no-op here: this controller's ContainerSpecs never
// set Volumes, so nothing in this package's tests exercises it. Added
// purely to keep fakeRuntime satisfying docker.Runtime as that interface
// grows, the same pattern ListImages and Events above already follow.
func (f *fakeRuntime) EnsureVolume(_ context.Context, _ string) error {
	return nil
}

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}

func (f *fakeRuntime) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

func (f *fakeRuntime) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for name := range f.containers {
		out = append(out, name)
	}
	return out
}

func alwaysHealthy() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func neverHealthy() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}

func conditionOf(t *testing.T, result reconcile.Result) reconcile.Condition {
	t.Helper()
	if len(result.Conditions) == 0 {
		t.Fatal("expected at least one condition, got none")
	}
	return result.Conditions[0]
}

func TestController_Reconcile_NoDesiredState(t *testing.T) {
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{err: store.ErrServiceNotFound}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (missing desired state is not a failure)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "NoDesiredState" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=NoDesiredState", cond)
	}
}

func TestController_Reconcile_StoreError(t *testing.T) {
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{err: errors.New("db unreachable")}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
}

func TestController_Reconcile_FreshDeploy_NoHealthCheck(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
}

func TestController_Reconcile_FreshDeploy_ReadinessSucceeds(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 200 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
}

func TestController_Reconcile_FreshDeploy_ReadinessFails(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(150*time.Millisecond))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a readiness timeout error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "ReadinessFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=ReadinessFailed", cond)
	}
}

func TestController_Reconcile_AlreadyRunning_NoOp(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	target := containerName("web", desired.Image)
	rt.seed(target, true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (already converged, must be a no-op)", rt.createCalls)
	}
}

func TestController_Reconcile_RestartAfterCrash(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	target := containerName("web", desired.Image)
	rt.seed(target, false) // exists, not running: crashed

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed (restart path re-verifies readiness)", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (a crashed container is restarted, not recreated)", rt.createCalls)
	}
}

func TestController_Reconcile_Redeploy_CleansUpOldContainer(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	oldTarget := containerName("web", "img:v1")
	rt.seed(oldTarget, true)

	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 200 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}

	newTarget := containerName("web", "img:v2")
	names := rt.names()
	if len(names) != 1 || names[0] != newTarget {
		t.Errorf("containers after redeploy = %v, want exactly [%s] (old must be cleaned up, only after new is healthy)", names, newTarget)
	}
}

func TestController_Reconcile_CleanupFailure_StillReportsReadyButErrors(t *testing.T) {
	rt := newFakeRuntime(0)
	oldTarget := containerName("web", "img:v1")
	rt.seed(oldTarget, true)
	rt.removeErr = errors.New("permission denied")

	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the cleanup error to surface")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition.Status = %v, want True: the new container IS healthy and serving despite cleanup failing", cond.Status)
	}
	if cond.Reason != "RunningStaleCleanupFailed" {
		t.Errorf("condition.Reason = %q, want RunningStaleCleanupFailed", cond.Reason)
	}
}

func TestController_Reconcile_NoPort_SkipsProbeEntirely(t *testing.T) {
	// A worker service: nothing listens on a port, so nothing to probe,
	// even if (inconsistently) a Health block were present.
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "worker", Image: "img:v1", Port: 0,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz"}},
	}
	c := New("worker", &fakeStore{svc: desired}, rt, WithReadyBudget(50*time.Millisecond))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (no port means nothing to probe)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
}

func TestController_Name(t *testing.T) {
	c := New("web", &fakeStore{}, newFakeRuntime(0))
	if got, want := c.Name(), "application/web"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Error("WithHTTPClient did not override the default client")
	}
}

func TestContainerName_DeterministicPerImage(t *testing.T) {
	a1 := containerName("web", "img:v1")
	a2 := containerName("web", "img:v1")
	b := containerName("web", "img:v2")

	if a1 != a2 {
		t.Errorf("containerName is not deterministic: %q != %q for the same inputs", a1, a2)
	}
	if a1 == b {
		t.Errorf("containerName collided for different images: %q", a1)
	}
	if !strings.HasPrefix(a1, "web-") {
		t.Errorf("containerName(%q) = %q, want it prefixed with the service name", "web", a1)
	}
}
