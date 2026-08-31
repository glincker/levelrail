package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// fakeVolumeRuntime is a hand-written fake docker.Runtime narrowed to
// what ContainerVolumeArchiver/ContainerVolumeRestorer actually exercise:
// Create, Start, Exec, ExecWithInput, Remove. Every other method is a
// compile-satisfying stub, the same convention fakeExecRuntime
// (dump_test.go) already establishes for this package's other fake.
type fakeVolumeRuntime struct {
	createErr error
	startErr  error
	execErr   error
	removeErr error

	gotCreateSpec docker.ContainerSpec
	createCalls   int
	startCalls    int
	removeCalls   int
	removedIDs    []string

	execContent string
	gotExecID   string
	gotExecCmd  []string

	gotInputID   string
	gotInputCmd  []string
	gotStdin     string
	execInputErr error
}

func (f *fakeVolumeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.createCalls++
	f.gotCreateSpec = spec
	if f.createErr != nil {
		return "", f.createErr
	}
	return "helper-" + spec.Name, nil
}

func (f *fakeVolumeRuntime) Start(_ context.Context, _ string) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeVolumeRuntime) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	f.gotExecID = containerID
	f.gotExecCmd = cmd
	if f.execErr != nil {
		return nil, f.execErr
	}
	return io.NopCloser(strings.NewReader(f.execContent)), nil
}

func (f *fakeVolumeRuntime) ExecWithInput(_ context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	f.gotInputID = containerID
	f.gotInputCmd = cmd
	b, _ := io.ReadAll(stdin)
	f.gotStdin = string(b)
	if f.execInputErr != nil {
		return nil, f.execInputErr
	}
	return io.NopCloser(strings.NewReader(f.execContent)), nil
}

func (f *fakeVolumeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.removeCalls++
	f.removedIDs = append(f.removedIDs, id)
	return f.removeErr
}

func (f *fakeVolumeRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeVolumeRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeVolumeRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeVolumeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeVolumeRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeVolumeRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakeVolumeRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakeVolumeRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeVolumeRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeVolumeRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func TestCreateVolumeHelper_MountsReadOnlyWhenAsked(t *testing.T) {
	rt := &fakeVolumeRuntime{}
	id, err := createVolumeHelper(context.Background(), rt, "app-web-data", "volbackup", true)
	if err != nil {
		t.Fatalf("createVolumeHelper() error = %v", err)
	}
	if id == "" {
		t.Fatal("createVolumeHelper() returned empty id")
	}
	if rt.startCalls != 1 {
		t.Errorf("Start calls = %d, want 1", rt.startCalls)
	}
	if len(rt.gotCreateSpec.Volumes) != 1 {
		t.Fatalf("create spec volumes = %+v, want exactly one mount", rt.gotCreateSpec.Volumes)
	}
	mount := rt.gotCreateSpec.Volumes[0]
	if mount.Name != "app-web-data" || mount.ContainerPath != volumeMountPath || !mount.ReadOnly {
		t.Errorf("mount = %+v, want {app-web-data %s true}", mount, volumeMountPath)
	}
}

func TestCreateVolumeHelper_StartFailure_RemovesContainer(t *testing.T) {
	rt := &fakeVolumeRuntime{startErr: errors.New("start failed")}
	_, err := createVolumeHelper(context.Background(), rt, "app-web-data", "volbackup", false)
	if err == nil {
		t.Fatal("createVolumeHelper() error = nil, want the Start failure")
	}
	if rt.removeCalls != 1 {
		t.Errorf("Remove calls after a Start failure = %d, want 1 (cleanup the half-created container)", rt.removeCalls)
	}
}
