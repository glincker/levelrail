package application

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
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

	createErr       error
	startErr        error
	stopErr         error
	removeErr       error
	ensureVolumeErr error
	// startErrOnce fails exactly the next Start call, then clears
	// itself: the "broken container, but recreating it works" scenario,
	// distinct from startErr's "every Start fails" shape.
	startErrOnce error

	createCalls       int
	removeCalls       int
	ensureVolumeCalls []string
	lastCreateEnv     map[string]string
	// lastCreateSpec is the full spec passed to the most recent Create
	// call, for assertions (like the mesh DNS wiring tests) that need
	// more than just Env.
	lastCreateSpec docker.ContainerSpec

	// networks, ensureNetworkCalls, ensureNetworkErr, removeNetworkErr,
	// and callOrder back the per-app networking tests: networks tracks
	// which names EnsureNetwork has (idempotently) created, callOrder
	// records "network:<name>" and "create:<name>" in the order this
	// fake actually saw them so a test can assert the network exists
	// before the container that needs it does.
	networks           map[string]string
	ensureNetworkCalls int
	ensureNetworkErr   error
	removeNetworkErr   error
	removedNetworks    []string
	callOrder          []string
}

func newFakeRuntime(hostPort int) *fakeRuntime {
	return &fakeRuntime{containers: map[string]*docker.ContainerState{}, networks: map[string]string{}, hostPort: hostPort}
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
	f.lastCreateEnv = spec.Env
	f.lastCreateSpec = spec
	f.callOrder = append(f.callOrder, "create:"+spec.Name)
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
	if f.startErrOnce != nil {
		err := f.startErrOnce
		f.startErrOnce = nil
		return err
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

func (f *fakeRuntime) EnsureVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureVolumeCalls = append(f.ensureVolumeCalls, name)
	return f.ensureVolumeErr
}

// EnsureNetwork is stateful, unlike EnsureVolume above: this package's
// own networking tests need to assert idempotency (calling it twice for
// the same name must not create it twice) and ordering (a network
// exists before the container that needs it is created), which a plain
// no-op can't exercise.
func (f *fakeRuntime) EnsureNetwork(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureNetworkCalls++
	if f.ensureNetworkErr != nil {
		return "", f.ensureNetworkErr
	}
	if id, ok := f.networks[name]; ok {
		return id, nil
	}
	f.callOrder = append(f.callOrder, "network:"+name)
	id := "net-" + name
	f.networks[name] = id
	return id, nil
}

func (f *fakeRuntime) RemoveNetwork(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeNetworkErr != nil {
		return f.removeNetworkErr
	}
	delete(f.networks, name)
	f.removedNetworks = append(f.removedNetworks, name)
	return nil
}

func (f *fakeRuntime) ListNetworksByPrefix(_ context.Context, prefix string) ([]docker.NetworkInfo, error) {
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

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}

// Exec is a no-op here for the same reason EnsureVolume above is: this
// controller never runs a command inside a container, only manages
// container lifecycle. internal/backup's Dumper is the real caller,
// covered by that package's own tests.
func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}

// ExecWithInput is the same no-op stub as Exec above, for the same
// reason: internal/backup's Restorer is the real caller, exercised by
// that package's own tests.
func (f *fakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented")
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
	target := ContainerName("web", desired.Image, "")
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
	target := ContainerName("web", desired.Image, "")
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

// TestController_Reconcile_RestartAfterCrash_EnsuresNetworkFirst covers
// the gap that let a stopped container become permanently stuck: its
// per-app network can go missing between reconcile passes (an operator
// running docker network prune, or any other external interference this
// codebase can't prevent), and Start alone can never recover from that,
// only EnsureNetwork followed by Start can.
func TestController_Reconcile_RestartAfterCrash_EnsuresNetworkFirst(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false) // exists, not running, its network may or may not still exist

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.ensureNetworkCalls != 1 {
		t.Errorf("ensureNetworkCalls = %d, want 1: a stopped container's network must be re-ensured before Start, not just created fresh", rt.ensureNetworkCalls)
	}
}

// TestController_Reconcile_RestartAfterCrash_EnsureNetworkFails_StartNeverCalled
// is the half-succeeded case CLAUDE.md's testing standard requires for
// this path: if the network can't be re-ensured, Start must never run
// against a container whose network isn't there, and the container must
// stay stopped rather than being reported as recovered.
func TestController_Reconcile_RestartAfterCrash_EnsureNetworkFails_StartNeverCalled(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.ensureNetworkErr = errors.New("network not found")
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "EnsureNetworkFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=EnsureNetworkFailed", cond)
	}
	state, _ := rt.InspectByName(context.Background(), target)
	if state.Running {
		t.Error("container reported Running after a failed EnsureNetwork; Start must never have been reached")
	}
}

// TestController_Reconcile_RestartAfterCrash_StartFails_RecreatesAndRecovers
// covers the case live testing found: EnsureNetwork succeeds (the
// network genuinely exists) but Start still fails on the existing
// container, e.g. one whose first-ever Start already failed and left it
// with no real network binding, unrecoverable by retrying Start no
// matter how many times the network itself is re-ensured. The
// controller must remove that broken container and recreate it fresh
// rather than reporting a permanent StartFailed forever.
func TestController_Reconcile_RestartAfterCrash_StartFails_RecreatesAndRecovers(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	rt.startErrOnce = errors.New("network not found")

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want the broken container recovered by recreation", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 (the broken container must be removed before recreating)", rt.removeCalls)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (recreated fresh after the broken Start)", rt.createCalls)
	}
	state, _ := rt.InspectByName(context.Background(), target)
	if state == nil || !state.Running {
		t.Errorf("state = %+v, want a running container after recovery", state)
	}
}

// TestController_Reconcile_RestartAfterCrash_StartFails_RemoveAlsoFails is
// the half-succeeded case CLAUDE.md's testing standard requires: if the
// broken container can't even be removed, the controller must report
// both failures clearly rather than silently losing the original Start
// error or claiming success.
func TestController_Reconcile_RestartAfterCrash_StartFails_RemoveAlsoFails(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	rt.startErrOnce = errors.New("network not found")
	rt.removeErr = errors.New("permission denied")

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StartFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=StartFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: must never attempt to recreate a container it couldn't remove", rt.createCalls)
	}
}

// TestController_Reconcile_RestartAfterCrash_StartFails_RecreateAlsoFails
// is the other half-succeeded case: the broken container is removed
// successfully, but the fresh Create also fails (e.g. the underlying
// problem is not this one container at all). Must report CreateFailed,
// not silently retry Start against a container that no longer exists.
func TestController_Reconcile_RestartAfterCrash_StartFails_RecreateAlsoFails(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	rt.startErrOnce = errors.New("network not found")
	rt.createErr = errors.New("docker daemon unavailable")

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1: the broken container must still be removed even though recreation then fails", rt.removeCalls)
	}
}

func TestController_Reconcile_Redeploy_CleansUpOldContainer(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	oldTarget := ContainerName("web", "img:v1", "")
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

	newTarget := ContainerName("web", "img:v2", "")
	names := rt.names()
	if len(names) != 1 || names[0] != newTarget {
		t.Errorf("containers after redeploy = %v, want exactly [%s] (old must be cleaned up, only after new is healthy)", names, newTarget)
	}
}

func TestController_Reconcile_Suspended_RemovesRunningContainers(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Suspended: true}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "Suspended" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=Suspended", cond)
	}
	if names := rt.names(); len(names) != 0 {
		t.Errorf("containers after suspend = %v, want none", names)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (suspend must never create a container)", rt.createCalls)
	}
}

func TestController_Reconcile_Suspended_RemoveFails_ReportsNotReady(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Suspended: true}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	rt.removeErr = errors.New("permission denied")

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the removal failure to surface: a suspend that failed to stop the container must not be reported as done")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "SuspendFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=SuspendFailed", cond)
	}
}

func TestController_Reconcile_Suspended_NoContainers_NoOp(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Suspended: true}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "Suspended" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=Suspended", cond)
	}
	if names := rt.names(); len(names) != 0 {
		t.Errorf("containers after suspend = %v, want none", names)
	}
}

func TestController_Reconcile_CleanupFailure_StillReportsReadyButErrors(t *testing.T) {
	rt := newFakeRuntime(0)
	oldTarget := ContainerName("web", "img:v1", "")
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

func TestController_Reconcile_DoesNotTouchDifferentServiceContainers(t *testing.T) {
	// internal/spec's service-name validation (nameLike) permits hyphens
	// freely, so "web" and "web-worker" are both valid, distinct service
	// names. ListByPrefix("web-") is a bare string-prefix match: without
	// an exact-ownership check, reconciling "web" would find
	// "web-worker"'s container (its name also starts with "web-") and
	// treat it as a stale leftover of "web" itself, stopping and
	// removing an entirely different, live service's container.
	rt := newFakeRuntime(0)
	otherServiceContainer := ContainerName("web-worker", "img:v9", "")
	rt.seed(otherServiceContainer, true)

	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}

	if rt.removeCalls != 0 {
		t.Errorf("removeCalls = %d, want 0: reconciling %q must never remove %q's container", rt.removeCalls, "web", "web-worker")
	}
	found := false
	for _, n := range rt.names() {
		if n == otherServiceContainer {
			found = true
		}
	}
	if !found {
		t.Errorf("containers after reconcile = %v, want %q (another service's container) still present", rt.names(), otherServiceContainer)
	}
}

func TestController_Reconcile_CreateSucceedsStartFails_HalfSucceeded(t *testing.T) {
	// The half-succeeded case this codebase's testing standard
	// explicitly requires a test for: create succeeds (a container now
	// exists) but start fails (it isn't running). This proves Reconcile
	// reports that honestly as a
	// failure, and that the next call recovers by restarting the
	// existing container rather than erroring forever or blindly
	// recreating it.
	rt := newFakeRuntime(0)
	rt.startErr = errors.New("start failed")

	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the start failure to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (create succeeded before start failed)", rt.createCalls)
	}

	rt.startErr = nil
	result2, err2 := c.Reconcile(context.Background())
	if err2 != nil {
		t.Fatalf("second Reconcile() error = %v, want nil: must recover from the half-succeeded state", err2)
	}
	cond2 := conditionOf(t, result2)
	if cond2.Status != reconcile.ConditionTrue || cond2.Reason != "Deployed" {
		t.Errorf("second reconcile condition = %+v, want Status=True Reason=Deployed (recovered from half-succeeded create)", cond2)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls after recovery = %d, want still 1: the half-created container must be restarted, not recreated", rt.createCalls)
	}
}

func TestController_Reconcile_Redeploy_ReadinessFails_OldContainerSurvives(t *testing.T) {
	// TestController_Reconcile_FreshDeploy_ReadinessFails covers a
	// from-scratch deploy with nothing to lose; this covers the
	// higher-stakes case: a redeploy whose new container never becomes
	// ready must never touch the old, still-serving container. Old is
	// stopped only after new is confirmed ready, never before or
	// instead.
	srv := neverHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seed(oldTarget, true)

	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
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

	if rt.removeCalls != 0 {
		t.Errorf("removeCalls = %d, want 0: the old container must never be removed when the new one's readiness probe fails", rt.removeCalls)
	}
	found := false
	for _, n := range rt.names() {
		if n == oldTarget {
			found = true
		}
	}
	if !found {
		t.Errorf("containers after failed redeploy = %v, want old container %q still present and serving", rt.names(), oldTarget)
	}
}

func TestOwnsContainer(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		container   string
		want        bool
	}{
		{name: "exact match", serviceName: "web", container: ContainerName("web", "img:v1", ""), want: true},
		{name: "different service that happens to prefix-match", serviceName: "web", container: ContainerName("web-worker", "img:v1", ""), want: false},
		{name: "unrelated name", serviceName: "web", container: "totally-unrelated", want: false},
		{name: "right prefix, wrong suffix length", serviceName: "web", container: "web-abc", want: false},
		{name: "right prefix, non-hex suffix", serviceName: "web", container: "web-zzzzzzzz", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ownsContainer(tt.serviceName, tt.container); got != tt.want {
				t.Errorf("ownsContainer(%q, %q) = %v, want %v", tt.serviceName, tt.container, got, tt.want)
			}
		})
	}
}

// fakeSecretResolver is a hand-written fake for SecretResolver, the same
// pattern every other fake in this file uses: a set of (service, key)
// pairs mapped to plaintext values, standing in for a real
// secrets.Manager without a master key or database.
type fakeSecretResolver struct {
	values       map[string]string
	existsErr    error
	resolveErr   error
	existsCalls  int
	resolveCalls int
}

func newFakeSecretResolver(values map[string]string) *fakeSecretResolver {
	return &fakeSecretResolver{values: values}
}

func (f *fakeSecretResolver) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	f.existsCalls++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.values[serviceName+"/"+envKey]
	return ok, nil
}

func (f *fakeSecretResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.resolveCalls++
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return f.values[serviceName+"/"+envKey], nil
}

func TestController_Reconcile_SecretEnv_NoResolverConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		SecretEnv: []string{"API_KEY"},
	}
	c := New("web", &fakeStore{svc: desired}, rt) // no WithSecretResolver

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a secret-backed env var with no resolver configured must never silently start a container missing it")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: must never call Create with an unresolved secret", rt.createCalls)
	}
}

func TestController_Reconcile_SecretEnv_Resolved_MergedIntoContainerEnv(t *testing.T) {
	rt := newFakeRuntime(0)
	resolver := newFakeSecretResolver(map[string]string{"web/API_KEY": "sk-real-value"})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env:       map[string]string{"NODE_ENV": "production"},
		SecretEnv: []string{"API_KEY"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}

	if got := rt.lastCreateEnv["API_KEY"]; got != "sk-real-value" {
		t.Errorf("container env API_KEY = %q, want the resolved secret value", got)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal value preserved alongside the resolved secret", got)
	}
}

func TestController_Reconcile_SecretEnv_OptionalUnsetSecret_OmittedNotFailed(t *testing.T) {
	rt := newFakeRuntime(0)
	resolver := newFakeSecretResolver(nil) // nothing set
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		SecretEnv: []string{"OPTIONAL_FLAG"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want an unset optional secret to be omitted, not fail reconcile", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	if _, exists := rt.lastCreateEnv["OPTIONAL_FLAG"]; exists {
		t.Error("container env contains OPTIONAL_FLAG, want it omitted since no value was ever set")
	}
}

func TestController_Reconcile_SecretEnv_ResolverErrorPropagates(t *testing.T) {
	rt := newFakeRuntime(0)
	resolver := newFakeSecretResolver(map[string]string{"web/API_KEY": "value"})
	resolver.resolveErr = errors.New("master key not configured")
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		SecretEnv: []string{"API_KEY"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(resolver))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the resolver's error to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_NoSecretEnv_ResolverNeverCalled(t *testing.T) {
	// A service with no secret-backed env vars must not pay any cost for
	// a configured resolver it doesn't need.
	rt := newFakeRuntime(0)
	resolver := newFakeSecretResolver(nil)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if resolver.existsCalls != 0 || resolver.resolveCalls != 0 {
		t.Errorf("resolver calls = exists:%d resolve:%d, want 0/0", resolver.existsCalls, resolver.resolveCalls)
	}
}

// TestController_ResolveEnv_SecretWinsOnKeyCollision pins down
// resolveEnv's merge precedence directly (rather than only observing it
// indirectly through Reconcile, as the tests above do): when
// desired.Env and desired.SecretEnv declare the same key, the
// secret-resolved value must win, deterministically, every time. See
// the doc comment on resolveEnv for why that's the intended precedence
// rather than an accident of map iteration order.
func TestController_ResolveEnv_SecretWinsOnKeyCollision(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		secretEnv []string
		secrets   map[string]string // "service/key" -> value, per fakeSecretResolver's convention
		want      map[string]string
	}{
		{
			name:      "same key in both: secret overwrites the literal",
			env:       map[string]string{"X": "plain"},
			secretEnv: []string{"X"},
			secrets:   map[string]string{"web/X": "secret-value"},
			want:      map[string]string{"X": "secret-value"},
		},
		{
			name:      "no key overlap: literal and secret both survive untouched",
			env:       map[string]string{"OTHER": "literal-value"},
			secretEnv: []string{"API_KEY"},
			secrets:   map[string]string{"web/API_KEY": "secret-value"},
			want:      map[string]string{"OTHER": "literal-value", "API_KEY": "secret-value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := newFakeSecretResolver(tt.secrets)
			desired := &store.DesiredService{
				Name:      "web",
				Env:       tt.env,
				SecretEnv: tt.secretEnv,
			}
			c := New("web", &fakeStore{}, newFakeRuntime(0), WithSecretResolver(resolver))

			got, err := c.resolveEnv(context.Background(), desired)
			if err != nil {
				t.Fatalf("resolveEnv() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("resolveEnv() = %v, want %v", got, tt.want)
			}
			for k, wantV := range tt.want {
				if gotV := got[k]; gotV != wantV {
					t.Errorf("resolveEnv()[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

// fakeStorageTargetStore is a hand-written fake for StorageTargetStore,
// the same pattern every other fake in this file uses: a set of
// pre-seeded store.BackupTarget rows, standing in for a real *store.DB
// without a real database.
type fakeStorageTargetStore struct {
	targets map[string]store.BackupTarget
	err     error
	calls   int
}

func (f *fakeStorageTargetStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	f.calls++
	if f.err != nil {
		return store.BackupTarget{}, f.err
	}
	target, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return target, nil
}

func TestController_Reconcile_NoStorageTarget_Unaffected(t *testing.T) {
	// A service with no StorageTargetID must reconcile identically
	// whether or not WithStorageTargets/WithSecretResolver are
	// configured, and must never pay any cost (a lookup call) for either.
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{}}
	secretResolver := newFakeSecretResolver(nil)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env: map[string]string{"NODE_ENV": "production"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore), WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	if storageStore.calls != 0 {
		t.Errorf("GetBackupTarget calls = %d, want 0: a service with no StorageTargetID must never look one up", storageStore.calls)
	}
	for _, key := range []string{"S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_ENDPOINT", "S3_REGION"} {
		if _, exists := rt.lastCreateEnv[key]; exists {
			t.Errorf("container env contains %s, want it absent: no storage target is attached", key)
		}
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal value preserved", got)
	}
}

func TestController_Reconcile_StorageTarget_Injected_MergedIntoContainerEnv(t *testing.T) {
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Name: "app-bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Region: "auto", Bucket: "app-data"},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKIA_REAL",
		"backup-target/bkt_1/secret_access_key": "sk-real-secret",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env:             map[string]string{"NODE_ENV": "production"},
		StorageTargetID: "bkt_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore), WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}

	want := map[string]string{
		"NODE_ENV":             "production",
		"S3_BUCKET":            "app-data",
		"S3_ENDPOINT":          "https://r2.example.com",
		"S3_REGION":            "auto",
		"S3_ACCESS_KEY_ID":     "AKIA_REAL",
		"S3_SECRET_ACCESS_KEY": "sk-real-secret",
	}
	for k, wantV := range want {
		if gotV := rt.lastCreateEnv[k]; gotV != wantV {
			t.Errorf("container env %s = %q, want %q", k, gotV, wantV)
		}
	}
}

// TestController_Reconcile_StorageTarget_AWSProviderOmitsEndpointAndRegion
// proves resolveStorageEnv's own doc comment: an "aws" provider target
// with no Endpoint/Region set (store.BackupTarget's own documented
// meaning for that provider) must not inject an empty S3_ENDPOINT/
// S3_REGION some S3-compatible client library would otherwise try to
// dial or misinterpret.
func TestController_Reconcile_StorageTarget_AWSProviderOmitsEndpointAndRegion(t *testing.T) {
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Name: "app-bucket", Provider: store.BackupProviderAWS, Bucket: "app-data"},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKIA_REAL",
		"backup-target/bkt_1/secret_access_key": "sk-real-secret",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		StorageTargetID: "bkt_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore), WithSecretResolver(secretResolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, key := range []string{"S3_ENDPOINT", "S3_REGION"} {
		if _, exists := rt.lastCreateEnv[key]; exists {
			t.Errorf("container env contains %s, want it absent for an aws-provider target with no explicit endpoint/region", key)
		}
	}
	if got := rt.lastCreateEnv["S3_BUCKET"]; got != "app-data" {
		t.Errorf("container env S3_BUCKET = %q, want app-data", got)
	}
}

func TestController_Reconcile_StorageTarget_NoStorageTargetStoreConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	secretResolver := newFakeSecretResolver(map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKIA_REAL",
		"backup-target/bkt_1/secret_access_key": "sk-real-secret",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		StorageTargetID: "bkt_1",
	}
	// No WithStorageTargets.
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a storage target with no store configured must never silently start a container missing its credentials")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_StorageTarget_NoSecretResolverConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Name: "app-bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Bucket: "app-data"},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		StorageTargetID: "bkt_1",
	}
	// No WithSecretResolver.
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a storage target with no secret resolver configured must never silently start a container missing its credentials")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_StorageTarget_UnknownTarget_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{}} // empty: bkt_1 doesn't exist
	secretResolver := newFakeSecretResolver(nil)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		StorageTargetID: "bkt_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore), WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the backup target lookup failure to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

// fakeRegistryCredentialStore is a hand-written fake for
// RegistryCredentialStore, the same pattern fakeStorageTargetStore
// above uses.
type fakeRegistryCredentialStore struct {
	credentials map[string]store.RegistryCredential
}

func (f *fakeRegistryCredentialStore) GetRegistryCredential(_ context.Context, id string) (store.RegistryCredential, error) {
	cred, ok := f.credentials[id]
	if !ok {
		return store.RegistryCredential{}, store.ErrRegistryCredentialNotFound
	}
	return cred, nil
}

func TestController_Reconcile_RegistryCredential_ResolvedIntoPullAuth(t *testing.T) {
	rt := newFakeRuntime(0)
	credStore := &fakeRegistryCredentialStore{credentials: map[string]store.RegistryCredential{
		"regcred_1": {ID: "regcred_1", Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot"},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{ //nolint:gosec // fake fixture, not a real credential
		"registry-credential/regcred_1/password": "gh-token-real",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "ghcr.io/org/app:v1", Port: 80,
		RegistryCredentialID: "regcred_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithRegistryCredentials(credStore), WithSecretResolver(secretResolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	auth := rt.lastCreateSpec.RegistryAuth
	if auth == nil {
		t.Fatal("lastCreateSpec.RegistryAuth = nil, want it set")
	}
	if auth.Username != "bot" || auth.Password != "gh-token-real" {
		t.Errorf("RegistryAuth = %+v, want Username=bot Password=gh-token-real", auth)
	}
}

func TestController_Reconcile_NoRegistryCredential_Unaffected(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "public/app:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.lastCreateSpec.RegistryAuth != nil {
		t.Errorf("RegistryAuth = %+v, want nil for a service with no RegistryCredentialID", rt.lastCreateSpec.RegistryAuth)
	}
}

func TestController_Reconcile_RegistryCredential_NoStoreConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	secretResolver := newFakeSecretResolver(map[string]string{ //nolint:gosec // fake fixture, not a real credential
		"registry-credential/regcred_1/password": "gh-token-real",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "ghcr.io/org/app:v1", Port: 80,
		RegistryCredentialID: "regcred_1",
	}
	// No WithRegistryCredentials.
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a registry credential with no store configured must never silently attempt an unauthenticated pull")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_RegistryCredential_NoSecretResolverConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	credStore := &fakeRegistryCredentialStore{credentials: map[string]store.RegistryCredential{
		"regcred_1": {ID: "regcred_1", Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot"},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "ghcr.io/org/app:v1", Port: 80,
		RegistryCredentialID: "regcred_1",
	}
	// No WithSecretResolver.
	c := New("web", &fakeStore{svc: desired}, rt, WithRegistryCredentials(credStore))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a failure: a registry credential with no secret resolver configured must never silently attempt an unauthenticated pull")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_RegistryCredential_Unknown_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	credStore := &fakeRegistryCredentialStore{credentials: map[string]store.RegistryCredential{}} // empty: regcred_1 doesn't exist
	secretResolver := newFakeSecretResolver(nil)
	desired := &store.DesiredService{
		Name: "web", Image: "ghcr.io/org/app:v1", Port: 80,
		RegistryCredentialID: "regcred_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithRegistryCredentials(credStore), WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the registry credential lookup failure to propagate")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

// TestController_ResolveEnv_StorageTargetWinsOnKeyCollision pins down
// resolveEnv's documented final-precedence rule directly: a storage
// target's resolved S3_* values must win over both a same-named literal
// env var and a same-named resolved secret, deterministically, every
// time.
func TestController_ResolveEnv_StorageTargetWinsOnKeyCollision(t *testing.T) {
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Bucket: "real-bucket"},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"web/S3_BUCKET":                         "secret-shadow-value",
		"backup-target/bkt_1/access_key_id":     "AKIA_REAL",
		"backup-target/bkt_1/secret_access_key": "sk-real-secret",
	})
	desired := &store.DesiredService{
		Name:            "web",
		Env:             map[string]string{"S3_BUCKET": "operator-typo-value"},
		SecretEnv:       []string{"S3_BUCKET"},
		StorageTargetID: "bkt_1",
	}
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithStorageTargets(storageStore), WithSecretResolver(secretResolver))

	got, err := c.resolveEnv(context.Background(), desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}
	if got["S3_BUCKET"] != "real-bucket" {
		t.Errorf("resolveEnv()[S3_BUCKET] = %q, want the storage target's own value (real-bucket) to win over both the literal and the secret", got["S3_BUCKET"])
	}
}

// fakeProjectEnvStore is a hand-written fake for ProjectEnvStore, same
// pattern as fakeStorageTargetStore above.
type fakeProjectEnvStore struct {
	vars  map[string]map[string]string
	err   error
	calls int
}

func (f *fakeProjectEnvStore) ListProjectEnvVars(_ context.Context, projectID string) (map[string]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.vars[projectID], nil
}

func TestController_Reconcile_NoProjectID_ProjectEnvNeverConsulted(t *testing.T) {
	rt := newFakeRuntime(0)
	projectEnv := &fakeProjectEnvStore{}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env: map[string]string{"NODE_ENV": "production"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithProjectEnv(projectEnv))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if projectEnv.calls != 0 {
		t.Errorf("ListProjectEnvVars calls = %d, want 0: a service with no ProjectID must never look one up", projectEnv.calls)
	}
}

func TestController_Reconcile_ProjectID_NoProjectEnvStoreConfigured_SkippedNotFailed(t *testing.T) {
	// Unlike SecretEnv/StorageTargetID, a ProjectID with no
	// WithProjectEnv configured is not a failure: a project is purely an
	// organizational label, see WithProjectEnv's own doc comment.
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env:       map[string]string{"NODE_ENV": "production"},
		ProjectID: "proj_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal value preserved", got)
	}
}

func TestController_Reconcile_ProjectEnv_MergedAsBaseLayer(t *testing.T) {
	rt := newFakeRuntime(0)
	projectEnv := &fakeProjectEnvStore{vars: map[string]map[string]string{
		"proj_1": {"LOG_LEVEL": "info", "NODE_ENV": "development"},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env:       map[string]string{"NODE_ENV": "production"},
		ProjectID: "proj_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithProjectEnv(projectEnv))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateEnv["LOG_LEVEL"]; got != "info" {
		t.Errorf("container env LOG_LEVEL = %q, want the project default (info)", got)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the service's own literal (production) to win over the project default", got)
	}
}

func TestController_Reconcile_ProjectEnv_ResolverErrorPropagates(t *testing.T) {
	rt := newFakeRuntime(0)
	projectEnv := &fakeProjectEnvStore{err: errors.New("db unavailable")}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		ProjectID: "proj_1",
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithProjectEnv(projectEnv))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the project env lookup failure to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

// fakeDeployRecorder is a hand-written fake for DeployRecorder, same
// pattern as fakeSecretResolver above.
type fakeDeployRecorder struct {
	err             error
	calls           int
	lastServiceName string
}

func (f *fakeDeployRecorder) RecordDeploy(_ context.Context, serviceName string, _ time.Time) error {
	f.calls++
	f.lastServiceName = serviceName
	return f.err
}

func TestController_Reconcile_FreshDeploy_RecordsDeployMetric(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	recorder := &fakeDeployRecorder{}
	c := New("web", &fakeStore{svc: desired}, rt, WithDeployRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if recorder.calls != 1 {
		t.Fatalf("RecordDeploy called %d times, want 1", recorder.calls)
	}
	if recorder.lastServiceName != "web" {
		t.Errorf("recorded service = %q, want web", recorder.lastServiceName)
	}
}

func TestController_Reconcile_AlreadyRunning_DoesNotRecordDeployMetric(t *testing.T) {
	// A no-op reconcile tick must never count as a deploy: RecordDeploy
	// firing here would make every resync interval look like a fresh
	// deploy, destroying the whole point of the metric.
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	recorder := &fakeDeployRecorder{}

	c := New("web", &fakeStore{svc: desired}, rt, WithDeployRecorder(recorder))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if recorder.calls != 0 {
		t.Errorf("RecordDeploy called %d times, want 0 (already converged, not a deploy)", recorder.calls)
	}
}

func TestController_Reconcile_NoRecorderConfigured_StillSucceeds(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt) // no WithDeployRecorder

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: no recorder configured is a valid default", err)
	}
}

func TestController_Reconcile_DeployRecorderError_SurfacesButStaysReady(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	recorder := &fakeDeployRecorder{err: errors.New("telemetry store unavailable")}
	c := New("web", &fakeStore{svc: desired}, rt, WithDeployRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the recorder's error to surface (for the engine to log)")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition.Status = %v, want True: the container really is deployed and running, a metrics-recording failure doesn't undo that", cond.Status)
	}
	if cond.Reason != "DeployedMetricRecordFailed" {
		t.Errorf("condition.Reason = %q, want DeployedMetricRecordFailed", cond.Reason)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1: the deploy itself must still have happened", rt.createCalls)
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
	a1 := ContainerName("web", "img:v1", "")
	a2 := ContainerName("web", "img:v1", "")
	b := ContainerName("web", "img:v2", "")

	if a1 != a2 {
		t.Errorf("ContainerName is not deterministic: %q != %q for the same inputs", a1, a2)
	}
	if a1 == b {
		t.Errorf("ContainerName collided for different images: %q", a1)
	}
	if !strings.HasPrefix(a1, "web-") {
		t.Errorf("ContainerName(%q) = %q, want it prefixed with the service name", "web", a1)
	}
}

func TestContainerName_EmptyNonce_IsAGoldenValue_NeverChanges(t *testing.T) {
	// The exact regression this task's own migration comment (0019_restart_nonce.sql)
	// promises: "every pre-existing, never-restarted service keeps the
	// exact container name it already has on upgrade". This is checked
	// against a literal golden string, not just "equals some other call
	// with the same inputs", because the whole point is that adding
	// restartNonce as a parameter must not have changed the hash function
	// itself for the empty-nonce case; a test that only compared two
	// fresh calls to each other would pass even if both had silently
	// drifted from what a real, already-running container was named
	// before this parameter existed.
	got := ContainerName("web", "img:v1", "")
	const want = "web-" + "5222ac0e" // sha256("img:v1")[:8], unchanged by this task
	if got != want {
		t.Errorf("ContainerName(\"web\", \"img:v1\", \"\") = %q, want golden value %q (an empty nonce must hash identically to before restartNonce existed)", got, want)
	}
}

func TestContainerName_NonEmptyNonce_ProducesADifferentName(t *testing.T) {
	same := ContainerName("web", "img:v1", "")
	nonce1 := ContainerName("web", "img:v1", "restart-nonce-1")
	nonce2 := ContainerName("web", "img:v1", "restart-nonce-2")

	if nonce1 == same {
		t.Errorf("a non-empty nonce produced the same name as no nonce at all: %q", nonce1)
	}
	if nonce1 == nonce2 {
		t.Errorf("two different nonces collided: %q", nonce1)
	}
	// Determinism must hold for a non-empty nonce too, the same as it
	// does for image alone (TestContainerName_DeterministicPerImage).
	if again := ContainerName("web", "img:v1", "restart-nonce-1"); again != nonce1 {
		t.Errorf("ContainerName is not deterministic for the same nonce: %q != %q", again, nonce1)
	}
}

func TestReplicaContainerName_Replica0MatchesContainerName(t *testing.T) {
	got := replicaContainerName("web", "img:v1", "", 0)
	want := ContainerName("web", "img:v1", "")
	if got != want {
		t.Errorf("replicaContainerName(_, _, 0) = %q, want exactly ContainerName's own output %q (backward compatibility for every existing single-replica service and internal/reconcile/ingress's own call to the exported ContainerName)", got, want)
	}
}

func TestReplicaContainerName_NonZeroIndexIsDistinctAndDeterministic(t *testing.T) {
	r0 := replicaContainerName("web", "img:v1", "", 0)
	r1 := replicaContainerName("web", "img:v1", "", 1)
	r1Again := replicaContainerName("web", "img:v1", "", 1)
	r2 := replicaContainerName("web", "img:v1", "", 2)

	if r1 == r0 {
		t.Errorf("replica 1's name must differ from replica 0's, got %q for both", r1)
	}
	if r1 != r1Again {
		t.Errorf("replicaContainerName is not deterministic for the same index: %q != %q", r1, r1Again)
	}
	if r1 == r2 {
		t.Errorf("replica 1 and replica 2 collided: %q", r1)
	}
}

func TestOwnsContainer_RecognizesReplicaSuffix(t *testing.T) {
	base := ContainerName("web", "img:v1", "") // e.g. "web-abcd1234"
	r1 := replicaContainerName("web", "img:v1", "", 1)
	r12 := replicaContainerName("web", "img:v1", "", 12)

	for _, name := range []string{base, r1, r12} {
		if !ownsContainer("web", name) {
			t.Errorf("ownsContainer(%q) = false, want true", name)
		}
	}
	if ownsContainer("web", base+"-rabc") {
		t.Error("ownsContainer accepted a non-numeric replica suffix")
	}
	if ownsContainer("web", base+"-r") {
		t.Error("ownsContainer accepted an empty replica index")
	}
	if ownsContainer("web-worker", base) {
		t.Error("ownsContainer matched a differently-named service's container (the hyphen-prefix collision this check exists to prevent)")
	}
}

func TestController_Reconcile_Replicas_FreshDeploy_CreatesAllAndKeepsAllRunning(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: store.DefaultDeployStrategy, Replicas: 3}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 3 {
		t.Errorf("createCalls = %d, want 3 (one per replica)", rt.createCalls)
	}
	if rt.count() != 3 {
		t.Errorf("container count = %d, want 3", rt.count())
	}
	for i := 0; i < 3; i++ {
		want := replicaContainerName("web", "img:v1", "", i)
		found := false
		for _, name := range rt.names() {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("replica %d container %q not found among %v", i, want, rt.names())
		}
	}
}

func TestController_Reconcile_Replicas_AlreadyRunning_NoOp(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: store.DefaultDeployStrategy, Replicas: 3}
	for i := 0; i < 3; i++ {
		rt.seed(replicaContainerName("web", "img:v1", "", i), true)
	}

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

func TestController_Reconcile_Replicas_ScaleDown_RemovesExcessEvenOnCurrentImage(t *testing.T) {
	// The scale-down case that isn't just "old image is stale": replica 2
	// runs the exact same, current image as replicas 0 and 1, but the
	// desired replica count dropped to 2, so it must still be removed.
	// staleContainers has no notion of "image is current", only "is this
	// name in the keep set", which is exactly what makes this case work.
	rt := newFakeRuntime(0)
	for i := 0; i < 3; i++ {
		rt.seed(replicaContainerName("web", "img:v1", "", i), true)
	}
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: store.DefaultDeployStrategy, Replicas: 2}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Status=True", cond)
	}
	if rt.count() != 2 {
		t.Errorf("container count = %d, want 2 (replica 2 must be removed as excess)", rt.count())
	}
	if _, ok := rt.containers[replicaContainerName("web", "img:v1", "", 2)]; ok {
		t.Error("excess replica 2 is still present, want it removed")
	}
}

func TestController_Reconcile_Recreate_FreshDeploy(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: store.DefaultDeployStrategy, Replicas: store.DefaultReplicas}
	desired.Strategy = "recreate"
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

func TestController_Reconcile_Recreate_AlreadyConverged_NeverStopsAnything(t *testing.T) {
	// The load-bearing idempotency case: reconcileRecreate's own doc
	// comment names this as the exact bug a naive
	// "always stop everything, then start everything" implementation
	// would have. A resync tick against an already-converged service
	// must not stop or remove a single container.
	rt := newFakeRuntime(0)
	target := ContainerName("web", "img:v1", "")
	rt.seed(target, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "recreate", Replicas: store.DefaultReplicas}

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
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
	if rt.removeCalls != 0 {
		t.Errorf("removeCalls = %d, want 0 (must never stop/remove an already-converged container)", rt.removeCalls)
	}
}

func TestController_Reconcile_Recreate_Redeploy_NoOverlapBetweenOldAndNew(t *testing.T) {
	// recreate's defining property versus blue-green: the old container
	// is gone before the new one is created, never coexisting even
	// briefly. Proven here by asserting exactly one container exists
	// throughout (the fake is synchronous, so this test can't observe an
	// instant of true overlap directly, but it can and does assert the
	// old name is gone and the new name is the only one present
	// afterward, which blue-green's own redeploy test already
	// distinguishes by contrast).
	rt := newFakeRuntime(0)
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seed(oldTarget, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "recreate", Replicas: store.DefaultReplicas}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.count() != 1 {
		t.Fatalf("container count = %d, want exactly 1", rt.count())
	}
	newTarget := ContainerName("web", "img:v2", "")
	if _, ok := rt.containers[newTarget]; !ok {
		t.Errorf("new-image container %q not found among %v", newTarget, rt.names())
	}
	if _, ok := rt.containers[oldTarget]; ok {
		t.Error("old-image container is still present, want it removed before the new one was created")
	}
}

func TestController_Reconcile_Recreate_CreateFailsAfterCleanup_HalfSucceeded(t *testing.T) {
	// The half-succeeded case this project's own testing standard calls
	// for: recreate has already torn down the old container (a real,
	// intentional consequence of this strategy, not a bug) by the time
	// creating the replacement fails. The next reconcile pass retries
	// create from a clean slate (nothing stale left to confuse it),
	// which is the correct, safe way to fail partway through this
	// specific strategy, but it must be a real, visible CreateFailed
	// condition, not a silently-swallowed error.
	rt := newFakeRuntime(0)
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seed(oldTarget, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "recreate", Replicas: store.DefaultReplicas}
	rt.createErr = errors.New("boom")

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a create failure")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if _, ok := rt.containers[oldTarget]; ok {
		t.Error("old container is still present; recreate's own doc comment says cleanup happens before create is attempted")
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 (old removed, new failed to create)", rt.count())
	}
}

func TestController_Reconcile_Recreate_SameNonce_AlreadyConverged(t *testing.T) {
	// The idempotency counterpart to the "different nonce forces
	// recreate" tests below: two reconcile passes with the SAME
	// RestartNonce (a resync tick, not a fresh restart request) must
	// stay a genuine no-op, the same guarantee
	// TestController_Reconcile_Recreate_AlreadyConverged_NeverStopsAnything
	// already proves for an ordinary redeploy.
	rt := newFakeRuntime(0)
	target := ContainerName("web", "img:v1", "restart-nonce-1")
	rt.seed(target, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "recreate", Replicas: store.DefaultReplicas, RestartNonce: "restart-nonce-1"}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 {
		t.Errorf("createCalls = %d, removeCalls = %d, want 0/0 (an unchanged nonce is not a restart request)", rt.createCalls, rt.removeCalls)
	}
}

func TestController_Reconcile_Recreate_RestartNonceChanged_ForcesRecreate(t *testing.T) {
	// The actual restart mechanism this task exists to add: a service
	// whose image never changed, but whose RestartNonce did (what
	// store.RestartService does), must not be treated as already
	// converged. This is the real regression the whole feature guards
	// against: before RestartNonce fed into ContainerName, re-saving the
	// exact same image was a genuine no-op, so there was no way to force
	// a restart at all.
	rt := newFakeRuntime(0)
	oldTarget := ContainerName("web", "img:v1", "restart-nonce-1")
	rt.seed(oldTarget, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "recreate", Replicas: store.DefaultReplicas, RestartNonce: "restart-nonce-2"}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	newTarget := ContainerName("web", "img:v1", "restart-nonce-2")
	if _, ok := rt.containers[newTarget]; !ok {
		t.Errorf("post-restart container %q not found among %v", newTarget, rt.names())
	}
	if _, ok := rt.containers[oldTarget]; ok {
		t.Error("pre-restart container is still present, want it removed")
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (a restart creates exactly one fresh container)", rt.createCalls)
	}
}

func TestController_Reconcile_BlueGreen_RestartNonceChanged_ForcesRecreate(t *testing.T) {
	// Same restart mechanism, proven against blue-green (the default
	// strategy, and the one most apps run under), mirroring
	// TestController_Reconcile_Redeploy_CleansUpOldContainer's own shape
	// for an image change: the new container is created and proven
	// healthy before the old one is removed, the same safety property a
	// restart gets for free by reusing this existing cutover logic
	// rather than a bespoke restart code path.
	srv := alwaysHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	oldTarget := ContainerName("web", "img:v1", "restart-nonce-1")
	rt.seed(oldTarget, true)

	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80, RestartNonce: "restart-nonce-2",
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

	newTarget := ContainerName("web", "img:v1", "restart-nonce-2")
	names := rt.names()
	if len(names) != 1 || names[0] != newTarget {
		t.Errorf("containers after restart = %v, want exactly [%s] (old removed only after new is healthy)", names, newTarget)
	}
}

func TestController_Reconcile_Strategy_Rolling_NotYetSupported(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "rolling", Replicas: store.DefaultReplicas}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (a permanently unsupported strategy is a known, documented state, not a transient failure)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StrategyNotSupported" {
		t.Errorf("condition = %+v, want Status=False Reason=StrategyNotSupported", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (must not touch the runtime at all for an unsupported strategy)", rt.createCalls)
	}
}

func TestController_Reconcile_Strategy_Unrecognized(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Strategy: "not-a-real-strategy", Replicas: store.DefaultReplicas}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StrategyUnrecognized" {
		t.Errorf("condition = %+v, want Status=False Reason=StrategyUnrecognized", cond)
	}
}

func TestController_Reconcile_EmptyStrategyAndZeroReplicas_DefaultsToBlueGreenSingleContainer(t *testing.T) {
	// A store.DesiredService{} constructed by hand (this package's own
	// tests before this change, and any future caller that bypasses
	// store.SaveDesiredService's own defaulting) must behave exactly
	// like today's single-container blue-green shape, per this
	// package's Reconcile's own doc comment on why this defaulting is
	// not redundant with the store layer's.
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80} // Strategy: "", Replicas: 0
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.count() != 1 {
		t.Errorf("container count = %d, want exactly 1 (Replicas: 0 must default to 1, not 0 replicas)", rt.count())
	}
	want := ContainerName("web", "img:v1", "")
	if _, ok := rt.containers[want]; !ok {
		t.Errorf("container %q not found, want the same name Strategy/Replicas being unset has always produced", want)
	}
}

func TestWithMeshDNSAddr_SetsContainerSpecDNS(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithMeshDNSAddr("172.17.0.1"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Status=True", cond)
	}
	want := []string{"172.17.0.1"}
	if got := rt.lastCreateSpec.DNS; !reflect.DeepEqual(got, want) {
		t.Errorf("created ContainerSpec.DNS = %+v, want %+v", got, want)
	}
}

// TestWithMeshDNSAddr_NotConfigured_LeavesDNSNil is the regression-safety
// half of the mesh DNS test: without WithMeshDNSAddr (the default, and
// every service before this option existed), Create must never see a
// non-nil DNS field. This is the test that would actually fail if the
// wiring accidentally set DNS unconditionally instead of gating on
// meshDNSAddr being configured.
func TestWithMeshDNSAddr_NotConfigured_LeavesDNSNil(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt) // no WithMeshDNSAddr

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.DNS; got != nil {
		t.Errorf("created ContainerSpec.DNS = %+v, want nil", got)
	}
}

// TestController_Reconcile_CustomLabels_LandOnCreatedContainer proves
// store.DesiredService.Labels (internal/spec.Service.Labels' resolved
// storage form) actually reaches docker.Runtime.Create, the same
// "assert on the fake's recorded ContainerSpec" pattern
// TestWithMeshDNSAddr_SetsContainerSpecDNS already establishes for DNS.
func TestController_Reconcile_CustomLabels_LandOnCreatedContainer(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Labels: map[string]string{"team": "platform", "tier": "frontend"},
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Status=True", cond)
	}
	want := map[string]string{"team": "platform", "tier": "frontend"}
	if got := rt.lastCreateSpec.Labels; !reflect.DeepEqual(got, want) {
		t.Errorf("created ContainerSpec.Labels = %+v, want %+v", got, want)
	}
}

// TestController_Reconcile_NoLabels_LeavesContainerSpecLabelsNil is the
// regression-safety counterpart, same reasoning
// TestWithMeshDNSAddr_NotConfigured_LeavesDNSNil documents for DNS: every
// service before this field existed must keep producing a nil
// ContainerSpec.Labels, not an accidental empty-but-non-nil map that
// would change what a real docker.Client.Create sends the Engine API.
func TestController_Reconcile_NoLabels_LeavesContainerSpecLabelsNil(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.Labels; got != nil {
		t.Errorf("created ContainerSpec.Labels = %+v, want nil", got)
	}
}

func TestController_Reconcile_Volumes_EnsuredAndMountedOnCreate(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Volumes: []store.ServiceVolume{
			{Name: "app-web-data", ContainerPath: "/var/lib/data"},
			{Name: "app-web-config", ContainerPath: "/etc/app"},
		},
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Status=True", cond)
	}

	wantEnsured := []string{"app-web-data", "app-web-config"}
	if !reflect.DeepEqual(rt.ensureVolumeCalls, wantEnsured) {
		t.Errorf("EnsureVolume calls = %v, want %v", rt.ensureVolumeCalls, wantEnsured)
	}
	wantMounts := []docker.VolumeMount{
		{Name: "app-web-data", ContainerPath: "/var/lib/data"},
		{Name: "app-web-config", ContainerPath: "/etc/app"},
	}
	if got := rt.lastCreateSpec.Volumes; !reflect.DeepEqual(got, wantMounts) {
		t.Errorf("created ContainerSpec.Volumes = %+v, want %+v", got, wantMounts)
	}
}

func TestController_Reconcile_Volumes_EnsureFails_CreateNeverCalled(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.ensureVolumeErr = errors.New("volume driver unavailable")
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Volumes: []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/var/lib/data"}},
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the volume ensure failure to propagate")
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse {
		t.Errorf("condition = %+v, want Status=False", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a container must never be created before its volumes exist", rt.createCalls)
	}
}

// TestController_Reconcile_NoVolumes_LeavesContainerSpecVolumesNil is the
// regression-safety counterpart, same reasoning
// TestController_Reconcile_NoLabels_LeavesContainerSpecLabelsNil gives
// for Labels: every service before this field existed must keep
// producing a nil ContainerSpec.Volumes.
func TestController_Reconcile_NoVolumes_LeavesContainerSpecVolumesNil(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.Volumes; got != nil {
		t.Errorf("created ContainerSpec.Volumes = %+v, want nil", got)
	}
	if rt.ensureVolumeCalls != nil {
		t.Errorf("EnsureVolume calls = %v, want none", rt.ensureVolumeCalls)
	}
}
