package agent

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// fakeRuntime is a minimal hand-written fake for docker.Runtime, the
// same "hand-written fake, not a mocking framework" convention every
// other package's tests in this codebase already follow. Only
// InspectByName is exercised meaningfully here: this package's own job
// is delegation, not container logic, so one representative method
// proving the call actually reaches the wrapped Runtime is enough; every
// other method just needs to compile against the interface.
type fakeRuntime struct {
	inspectCalls int
	lastName     string
	state        *docker.ContainerState
	err          error
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.inspectCalls++
	f.lastName = name
	return f.state, f.err
}
func (f *fakeRuntime) Create(context.Context, docker.ContainerSpec) (string, error) { return "", nil }
func (f *fakeRuntime) Start(context.Context, string) error                          { return nil }
func (f *fakeRuntime) Events(context.Context) (<-chan docker.Event, <-chan error)   { return nil, nil }
func (f *fakeRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeRuntime) Remove(context.Context, string, bool) error        { return nil }
func (f *fakeRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakeRuntime) EnsureVolume(context.Context, string) error            { return nil }
func (f *fakeRuntime) EnsureNetwork(context.Context, string) (string, error) { return "", nil }
func (f *fakeRuntime) RemoveNetwork(context.Context, string) error           { return nil }
func (f *fakeRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}
func (f *fakeRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented")
}

func TestLocal_DelegatesToWrappedRuntime(t *testing.T) {
	rt := &fakeRuntime{state: &docker.ContainerState{Name: "web-abc123", Running: true}}
	var transport Transport = NewLocal(rt)

	got, err := transport.InspectByName(context.Background(), "web-abc123")
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if rt.inspectCalls != 1 {
		t.Errorf("inspectCalls = %d, want 1: Local must delegate, not reimplement", rt.inspectCalls)
	}
	if rt.lastName != "web-abc123" {
		t.Errorf("lastName = %q, want web-abc123", rt.lastName)
	}
	if got == nil || got.Name != "web-abc123" {
		t.Errorf("got = %+v, want the fake's state returned unchanged", got)
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	rt := &fakeRuntime{}
	local := NewLocal(rt)

	reg.Register("node-1", local)

	got, err := reg.Get("node-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != Transport(local) {
		t.Errorf("Get() = %v, want the exact registered Transport back", got)
	}
}

func TestRegistry_Get_NotRegistered(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get("nonexistent")
	if !errors.Is(err, ErrNodeNotRegistered) {
		t.Errorf("Get() error = %v, want ErrNodeNotRegistered", err)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	reg.Register("node-1", NewLocal(&fakeRuntime{}))

	reg.Unregister("node-1")

	_, err := reg.Get("node-1")
	if !errors.Is(err, ErrNodeNotRegistered) {
		t.Errorf("Get() after Unregister() error = %v, want ErrNodeNotRegistered", err)
	}
}

func TestRegistry_Unregister_NeverRegistered_NotAnError(_ *testing.T) {
	reg := NewRegistry()
	reg.Unregister("nonexistent") // must not panic or otherwise fail
}

func TestRegistry_Register_ReplacesExisting(t *testing.T) {
	reg := NewRegistry()
	first := NewLocal(&fakeRuntime{})
	second := NewLocal(&fakeRuntime{})

	reg.Register("node-1", first)
	reg.Register("node-1", second)

	got, err := reg.Get("node-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != Transport(second) {
		t.Error("Get() after re-registering, want the second (replacing) Transport, not the first")
	}
}
