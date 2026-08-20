package cloudflaretunnel

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, the same pattern
// internal/reconcile/database's own fakeStore establishes.
type fakeStore struct {
	settings store.CloudflareTunnelSettings
	err      error
}

func (f *fakeStore) GetCloudflareTunnelSettings(_ context.Context) (store.CloudflareTunnelSettings, error) {
	if f.err != nil {
		return store.CloudflareTunnelSettings{}, f.err
	}
	return f.settings, nil
}

// fakeTokens is a hand-written fake for TokenResolver.
type fakeTokens struct {
	value      string
	set        bool
	existsErr  error
	resolveErr error
}

func (f *fakeTokens) Exists(_ context.Context, _, _ string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.set, nil
}

func (f *fakeTokens) Resolve(_ context.Context, _, _ string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	if !f.set {
		return "", errors.New("not found")
	}
	return f.value, nil
}

// fakeRuntime is a stateful fake, the same pattern
// internal/reconcile/database's own fakeRuntime establishes, trimmed to
// what this controller actually calls (no volumes/networks).
type fakeRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	nextID     int

	createErr error
	startErr  error
	stopErr   error
	removeErr error

	createCalls    int
	removeCalls    int
	lastCreateSpec docker.ContainerSpec
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{containers: map[string]*docker.ContainerState{}}
}

// seed always seeds under "acme-cloudflared": every test that calls it
// also uses WithContainerPrefix("acme").
func (f *fakeRuntime) seed(image string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	const name = "acme-cloudflared"
	f.containers[name] = &docker.ContainerState{ID: strconv.Itoa(f.nextID), Name: name, Image: image, Running: running}
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
	f.lastCreateSpec = spec
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

func (f *fakeRuntime) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, _ string) ([]docker.ContainerState, error) {
	return nil, nil
}

func (f *fakeRuntime) ListImages(_ context.Context, _ string) ([]docker.ImageInfo, error) {
	return nil, nil
}

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, _ string) error { return nil }

func (f *fakeRuntime) EnsureNetwork(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (f *fakeRuntime) RemoveNetwork(_ context.Context, _ string) error { return nil }

func (f *fakeRuntime) ListNetworksByPrefix(_ context.Context, _ string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}

func (f *fakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented")
}

func conditionOf(t *testing.T, result reconcile.Result) reconcile.Condition {
	t.Helper()
	if len(result.Conditions) == 0 {
		t.Fatal("expected at least one condition, got none")
	}
	return result.Conditions[0]
}

func TestReconcile_DisabledNoToken_NoContainer(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: false}}, &fakeTokens{}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0", rt.count())
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "Disabled" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=Disabled", cond)
	}
}

func TestReconcile_EnabledNoToken_NoContainer_ReportsFalse(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, &fakeTokens{}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0", rt.count())
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "TokenNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=TokenNotConfigured", cond)
	}
}

func TestReconcile_EnabledWithToken_CreatesAndStartsContainer(t *testing.T) {
	rt := newFakeRuntime()
	tokens := &fakeTokens{value: "super-secret-token", set: true}
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, tokens, rt, WithContainerPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
	state, _ := rt.InspectByName(context.Background(), "acme-cloudflared")
	if state == nil || !state.Running {
		t.Fatalf("container %+v, want a running acme-cloudflared container", state)
	}
	if rt.lastCreateSpec.Env[tunnelEnvKey] != "super-secret-token" {
		t.Errorf("TUNNEL_TOKEN env = %q, want the resolved token", rt.lastCreateSpec.Env[tunnelEnvKey])
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Connected" {
		t.Errorf("condition = %+v, want Status=True Reason=Connected", cond)
	}
}

func TestReconcile_AlreadyRunning_Idempotent_NoRecreate(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, true)
	tokens := &fakeTokens{value: "tok", set: true}
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, tokens, rt, WithContainerPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (already running, already-desired image)", rt.createCalls)
	}
	if rt.removeCalls != 0 {
		t.Errorf("removeCalls = %d, want 0", rt.removeCalls)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Connected" {
		t.Errorf("condition = %+v, want Status=True Reason=Connected", cond)
	}
}

func TestReconcile_Stopped_Restarts(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, false)
	tokens := &fakeTokens{value: "tok", set: true}
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, tokens, rt, WithContainerPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (restart, not recreate)", rt.createCalls)
	}
	state, _ := rt.InspectByName(context.Background(), "acme-cloudflared")
	if state == nil || !state.Running {
		t.Fatalf("container %+v, want running after restart", state)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
}

func TestReconcile_DisabledWhileRunning_RemovesContainer(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, true)
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: false}}, &fakeTokens{}, rt, WithContainerPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 after disabling", rt.count())
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "Disabled" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=Disabled", cond)
	}
}

func TestReconcile_NilTokenResolver_TreatedAsNoToken(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, nil, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 with no secrets manager configured", rt.count())
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "TokenNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=TokenNotConfigured", cond)
	}
}

func TestReconcile_StoreError(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{err: errors.New("db unreachable")}, &fakeTokens{}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
}

func TestReconcile_ImageChanged_ReplacesContainer(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed("cloudflare/cloudflared:2023.1.0", true)
	tokens := &fakeTokens{value: "tok", set: true}
	c := New(&fakeStore{settings: store.CloudflareTunnelSettings{Enabled: true}}, tokens, rt, WithContainerPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 (old image replaced)", rt.removeCalls)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
	state, _ := rt.InspectByName(context.Background(), "acme-cloudflared")
	if state == nil || state.Image != image {
		t.Fatalf("container %+v, want image = %q", state, image)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
}

func TestContainerName_DefaultsWhenPrefixEmpty(t *testing.T) {
	if got, want := ContainerName(""), "platform-cloudflared"; got != want {
		t.Errorf("ContainerName(\"\") = %q, want %q", got, want)
	}
	if got, want := ContainerName("Levelrail"), "Levelrail-cloudflared"; got != want {
		t.Errorf("ContainerName(%q) = %q, want %q", "Levelrail", got, want)
	}
}
