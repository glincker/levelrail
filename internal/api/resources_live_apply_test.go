package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestApplyResourcesLive_NoRuntimeConfigured(t *testing.T) {
	rt, _ := newTestRouter(t) // no WithExecRuntime
	if applied := rt.applyResourcesLive(context.Background(), "", "some-container", nil); applied {
		t.Error("applyResourcesLive() = true with no runtime resolver configured, want false")
	}
}

func TestApplyResourcesLive_ContainerNotRunning(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: false}}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	if applied := rt.applyResourcesLive(context.Background(), "", "app-web", &store.ServiceResources{MemoryBytes: 256 << 20}); applied {
		t.Error("applyResourcesLive() = true for a non-running container, want false")
	}
	if fake.updateResourcesCalls != 0 {
		t.Errorf("updateResourcesCalls = %d, want 0: must not call UpdateResources on a container that isn't running", fake.updateResourcesCalls)
	}
}

func TestApplyResourcesLive_ContainerMissing(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: nil}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	if applied := rt.applyResourcesLive(context.Background(), "", "app-web", &store.ServiceResources{MemoryBytes: 256 << 20}); applied {
		t.Error("applyResourcesLive() = true for a missing container, want false")
	}
}

func TestApplyResourcesLive_Success(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	resources := &store.ServiceResources{MemoryBytes: 512 << 20, NanoCPUs: 500_000_000, SwapMemoryBytes: 1 << 30, CPUSetCPUs: "0-1"}
	if applied := rt.applyResourcesLive(context.Background(), "", "app-web", resources); !applied {
		t.Fatal("applyResourcesLive() = false, want true")
	}
	want := docker.Resources{MemoryBytes: 512 << 20, NanoCPUs: 500_000_000, SwapMemoryBytes: 1 << 30, CPUSetCPUs: "0-1"}
	if fake.updateResourcesID != "c1" || fake.updateResourcesResources != want {
		t.Errorf("updateResourcesID=%q resources=%+v, want c1/%+v", fake.updateResourcesID, fake.updateResourcesResources, want)
	}
}

// TestApplyResourcesLive_NilResourcesClearsLimits covers the "operator
// removes every limit" case: a nil *store.ServiceResources must still
// reach UpdateResources, with a zero-value docker.Resources (Docker's
// own "no limit" shape), not be treated as "nothing to do." Clearing a
// limit is as real a change as setting one.
func TestApplyResourcesLive_NilResourcesClearsLimits(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	if applied := rt.applyResourcesLive(context.Background(), "", "app-web", nil); !applied {
		t.Fatal("applyResourcesLive() = false, want true")
	}
	if fake.updateResourcesResources != (docker.Resources{}) {
		t.Errorf("updateResourcesResources = %+v, want zero value", fake.updateResourcesResources)
	}
}

// TestApplyResourcesLive_UpdateResourcesFails covers the half-succeeded
// case: the container is found and running, but the Engine API call
// itself fails (e.g. an invalid cpuset for this host). applyResourcesLive
// must report false rather than panicking or silently claiming success,
// so the caller's response accurately says "not applied live" and the
// existing recreate-on-next-deploy path remains the source of truth.
func TestApplyResourcesLive_UpdateResourcesFails(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState:       &docker.ContainerState{ID: "c1", Running: true},
		updateResourcesErr: errors.New("invalid cpuset"),
	}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	if applied := rt.applyResourcesLive(context.Background(), "", "app-web", &store.ServiceResources{MemoryBytes: 256 << 20}); applied {
		t.Error("applyResourcesLive() = true despite UpdateResources failing, want false")
	}
}

func TestApplyResourcesLiveToReplicas_MultipleReplicas(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, _ := newTestRouterWithExecRuntime(t, fake)

	desired := store.DesiredService{Name: "web", Image: "web:v1", Replicas: 3, Resources: &store.ServiceResources{MemoryBytes: 256 << 20}}
	if applied := rt.applyResourcesLiveToReplicas(context.Background(), desired); !applied {
		t.Fatal("applyResourcesLiveToReplicas() = false, want true")
	}
	if fake.updateResourcesCalls != 3 {
		t.Errorf("updateResourcesCalls = %d, want 3 (one per replica)", fake.updateResourcesCalls)
	}
}

// TestApplyResourcesLiveToReplicas_PartiallyRunning covers a rolling
// deploy still in flight: one replica up, others not created yet. The
// running replica must still get the live update; the missing ones are
// not an error, they will get the new limits at create time regardless.
func TestApplyResourcesLiveToReplicas_PartiallyRunning(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "web:v1", Replicas: 2, Resources: &store.ServiceResources{MemoryBytes: 256 << 20}}
	names := desiredServiceContainerNames(desired)
	if len(names) != 2 {
		t.Fatalf("desiredServiceContainerNames returned %d names, want 2", len(names))
	}

	fake := &partialFakeRuntime{running: names[0]}
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))

	if applied := rt.applyResourcesLiveToReplicas(context.Background(), desired); !applied {
		t.Fatal("applyResourcesLiveToReplicas() = false, want true (one replica is running)")
	}
	if fake.updateCalls != 1 {
		t.Errorf("updateCalls = %d, want 1: only the running replica should be updated", fake.updateCalls)
	}
}

// partialFakeRuntime reports exactly one container name as running,
// every other name as not found, for
// TestApplyResourcesLiveToReplicas_PartiallyRunning. Embeds
// fakeExecAppRuntime for every method that test doesn't exercise.
type partialFakeRuntime struct {
	fakeExecAppRuntime
	running     string
	updateCalls int
}

func (f *partialFakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	if name != f.running {
		return nil, nil
	}
	return &docker.ContainerState{ID: "c-" + name, Running: true}, nil
}

func (f *partialFakeRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	f.updateCalls++
	return nil
}
