package registry

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, the same pattern
// cloudflaretunnel's own fakeStore establishes.
type fakeStore struct {
	settings store.RegistrySettings
	err      error
}

func (f *fakeStore) GetRegistrySettings(_ context.Context) (store.RegistrySettings, error) {
	if f.err != nil {
		return store.RegistrySettings{}, f.err
	}
	return f.settings, nil
}

// fakeCreds is a hand-written fake for CredentialResolver, the same
// pattern cloudflaretunnel's own fakeTokens establishes.
type fakeCreds struct {
	value      string
	set        bool
	existsErr  error
	resolveErr error
}

func (f *fakeCreds) Exists(_ context.Context, _, _ string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.set, nil
}

func (f *fakeCreds) Resolve(_ context.Context, _, _ string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	if !f.set {
		return "", errors.New("not found")
	}
	return f.value, nil
}

// fakeRuntime is a stateful fake, the same pattern
// internal/reconcile/database's own fakeRuntime establishes.
type fakeRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	volumes    map[string]bool
	nextID     int

	createErr       error
	startErr        error
	stopErr         error
	removeErr       error
	ensureVolumeErr error

	createCalls       int
	removeCalls       int
	ensureVolumeCalls int
	lastCreateSpec    docker.ContainerSpec
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{containers: map[string]*docker.ContainerState{}, volumes: map[string]bool{}}
}

// seed always seeds under "acme-registry": every test that calls it also
// uses WithContainerPrefix("acme"), the same pattern
// cloudflaretunnel's own fakeRuntime.seed establishes.
func (f *fakeRuntime) seed(image string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	const name = "acme-registry"
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

func (f *fakeRuntime) UpdateResources(_ context.Context, _ string, _ docker.Resources) error {
	return nil
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureVolumeCalls++
	if f.ensureVolumeErr != nil {
		return f.ensureVolumeErr
	}
	f.volumes[name] = true
	return nil
}

func (f *fakeRuntime) EnsureNetwork(_ context.Context, _ string) (string, error) { return "", nil }
func (f *fakeRuntime) RemoveNetwork(_ context.Context, _ string) error           { return nil }
func (f *fakeRuntime) ListNetworksByPrefix(_ context.Context, _ string) ([]docker.NetworkInfo, error) {
	return nil, nil
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

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}

func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}

func (f *fakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented")
}

func (f *fakeRuntime) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

func (f *fakeRuntime) hasVolume(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.volumes[name]
}

func TestReconcile_Disabled_NoContainer_StaysAbsent(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{settings: store.RegistrySettings{Enabled: false}}, &fakeCreds{}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
	assertCondition(t, res, reconcile.ConditionUnknown, "Disabled")
}

func TestReconcile_Disabled_RemovesExistingContainer(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, true)
	c := New(&fakeStore{settings: store.RegistrySettings{Enabled: false}}, &fakeCreds{}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1", rt.removeCalls)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 after disabling", rt.count())
	}
	assertCondition(t, res, reconcile.ConditionUnknown, "Disabled")
}

func TestReconcile_EnabledNoCredentials_BlockedLoudly(t *testing.T) {
	rt := newFakeRuntime()
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: false}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 without credentials", rt.createCalls)
	}
	assertCondition(t, res, reconcile.ConditionFalse, "CredentialsNotConfigured")
}

func TestReconcile_EnabledWithCredentials_CreatesVolumeAndContainer(t *testing.T) {
	rt := newFakeRuntime()
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", rt.createCalls)
	}
	if !rt.hasVolume("acme-registry-data") {
		t.Errorf("expected volume %q to be ensured", "acme-registry-data")
	}
	spec := rt.lastCreateSpec
	if spec.Name != "acme-registry" {
		t.Errorf("spec.Name = %q, want %q", spec.Name, "acme-registry")
	}
	if len(spec.Ports) != 1 || spec.Ports[0].HostPort != HostPort || spec.Ports[0].ContainerPort != containerPort {
		t.Errorf("spec.Ports = %+v, want a single binding %d -> %d", spec.Ports, containerPort, HostPort)
	}
	if spec.Env[htpasswdUserEnv] != "levelrail" {
		t.Errorf("spec.Env[%s] = %q, want %q", htpasswdUserEnv, spec.Env[htpasswdUserEnv], "levelrail")
	}
	if spec.Env[htpasswdHashEnv] == "" || spec.Env[htpasswdHashEnv] == "s3cret" {
		t.Errorf("spec.Env[%s] = %q, want a bcrypt hash, not empty or the raw password", htpasswdHashEnv, spec.Env[htpasswdHashEnv])
	}
	if len(spec.Entrypoint) == 0 {
		t.Errorf("spec.Entrypoint is empty, want an htpasswd boot override")
	}
	assertCondition(t, res, reconcile.ConditionTrue, "Running")
}

func TestReconcile_AlreadyRunning_NoRecreate(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, true)
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 for an already-running container", rt.createCalls)
	}
	assertCondition(t, res, reconcile.ConditionTrue, "Running")
}

func TestReconcile_StoppedContainer_Restarted(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(image, false)
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (start, not recreate)", rt.createCalls)
	}
	assertCondition(t, res, reconcile.ConditionTrue, "Running")
}

func TestReconcile_ImageChanged_Replaces(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed("registry:1", true)
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.removeCalls != 1 || rt.createCalls != 1 {
		t.Errorf("removeCalls/createCalls = %d/%d, want 1/1 for an image change", rt.removeCalls, rt.createCalls)
	}
	assertCondition(t, res, reconcile.ConditionTrue, "Running")
}

// TestReconcile_CreateFailsHalfway covers the "operation half-succeeded"
// case reconciler tests must cover: a Create failure
// must be reported loudly, not silently swallowed, and must not corrupt
// state.
func TestReconcile_CreateFailsHalfway(t *testing.T) {
	rt := newFakeRuntime()
	rt.createErr = errors.New("engine unreachable")
	settings := store.RegistrySettings{Enabled: true, Host: "registry.example", Username: "levelrail"}
	c := New(&fakeStore{settings: settings}, &fakeCreds{set: true, value: "s3cret"}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a create failure")
	}
	assertCondition(t, res, reconcile.ConditionFalse, "CreateFailed")
}

func TestReconcile_StoreError_ReturnsNotReady(t *testing.T) {
	rt := newFakeRuntime()
	c := New(&fakeStore{err: errors.New("db down")}, &fakeCreds{}, rt, WithContainerPrefix("acme"))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a store error")
	}
	assertCondition(t, res, reconcile.ConditionFalse, "StoreError")
}

func TestContainerName_VolumeName_DefaultPrefix(t *testing.T) {
	if got := ContainerName(""); got != "platform-registry" {
		t.Errorf("ContainerName(\"\") = %q, want %q", got, "platform-registry")
	}
	if got := VolumeName(""); got != "platform-registry-data" {
		t.Errorf("VolumeName(\"\") = %q, want %q", got, "platform-registry-data")
	}
}

func assertCondition(t *testing.T, res reconcile.Result, status reconcile.ConditionStatus, reason string) {
	t.Helper()
	for _, c := range res.Conditions {
		if c.Type != "Ready" {
			continue
		}
		if c.Status != status || c.Reason != reason {
			t.Errorf("Ready condition = %s/%s, want %s/%s", c.Status, c.Reason, status, reason)
		}
		return
	}
	t.Fatalf("no Ready condition in result: %+v", res.Conditions)
}
