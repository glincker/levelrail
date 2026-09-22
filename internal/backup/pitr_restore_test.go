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

// recordingHelperRuntime is a minimal docker.Runtime for
// ContainerPITRRestorer's own tests: ExecWithInput appends every call
// rather than overwriting a single "last call" field (fakeVolumeRuntime's
// own shape, volume_helper_test.go), since Restore issues two
// ExecWithInput calls against the same helper container (wipe+extract,
// then write the recovery config) and these tests need to inspect both.
type recordingHelperRuntime struct {
	createErr    error
	execInputErr error

	createCalls int
	removeCalls int
	lastSpec    docker.ContainerSpec
	execInputs  []recordedExecInput
}

type recordedExecInput struct {
	containerID string
	cmd         []string
	stdin       string
}

func (f *recordingHelperRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.createCalls++
	f.lastSpec = spec
	if f.createErr != nil {
		return "", f.createErr
	}
	return "helper-" + spec.Name, nil
}

func (f *recordingHelperRuntime) Start(context.Context, string) error { return nil }

func (f *recordingHelperRuntime) ExecWithInput(_ context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(stdin)
	f.execInputs = append(f.execInputs, recordedExecInput{containerID: containerID, cmd: cmd, stdin: string(b)})
	if f.execInputErr != nil {
		return nil, f.execInputErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *recordingHelperRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *recordingHelperRuntime) Remove(context.Context, string, bool) error {
	f.removeCalls++
	return nil
}

func (f *recordingHelperRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return nil, nil
}
func (f *recordingHelperRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *recordingHelperRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *recordingHelperRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *recordingHelperRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *recordingHelperRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *recordingHelperRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *recordingHelperRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *recordingHelperRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *recordingHelperRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func TestContainerPITRRestorer_Restore_WipesExtractsAndConfigures(t *testing.T) {
	rt := &recordingHelperRuntime{}
	r := &ContainerPITRRestorer{Runtime: rt}

	target := time.Date(2026, 8, 14, 6, 30, 0, 0, time.UTC)
	if err := r.Restore(context.Background(), "db-mydb-data", strings.NewReader("tar-bytes"), target); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 (one helper container)", rt.createCalls)
	}
	if rt.lastSpec.Volumes[0].Name != "db-mydb-data" || rt.lastSpec.Volumes[0].ReadOnly {
		t.Errorf("helper volume mount = %+v, want db-mydb-data read-write", rt.lastSpec.Volumes[0])
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 (helper cleaned up)", rt.removeCalls)
	}
	if len(rt.execInputs) != 2 {
		t.Fatalf("ExecWithInput calls = %d, want 2 (wipe+extract, then write recovery config)", len(rt.execInputs))
	}

	extract := rt.execInputs[0]
	if extract.stdin != "tar-bytes" {
		t.Errorf("extract stdin = %q, want tar-bytes", extract.stdin)
	}
	joined := strings.Join(extract.cmd, " ")
	if !strings.Contains(joined, "rm -rf") || !strings.Contains(joined, "tar -xf") {
		t.Errorf("extract command = %q, want a wipe followed by a tar extract", joined)
	}
	if !strings.Contains(joined, "chown -R 999:999") {
		t.Errorf("extract command = %q, want a chown to the postgres uid/gid", joined)
	}

	conf := rt.execInputs[1]
	if !strings.Contains(strings.Join(conf.cmd, " "), pitrConfFile) {
		t.Errorf("recovery config command = %q, want it to reference %q", conf.cmd, pitrConfFile)
	}
}

func TestContainerPITRRestorer_writeRecoveryConfig_ConfContent(t *testing.T) {
	rt := &recordingHelperRuntime{}
	r := &ContainerPITRRestorer{Runtime: rt}

	target := time.Date(2026, 8, 14, 6, 30, 15, 123456000, time.UTC)
	if err := r.writeRecoveryConfig(context.Background(), "helper-1", target); err != nil {
		t.Fatalf("writeRecoveryConfig() error = %v", err)
	}

	if len(rt.execInputs) != 1 {
		t.Fatalf("ExecWithInput calls = %d, want 1", len(rt.execInputs))
	}
	call := rt.execInputs[0]
	if call.containerID != "helper-1" {
		t.Errorf("ExecWithInput container = %q, want helper-1", call.containerID)
	}

	conf := call.stdin
	if !strings.Contains(conf, "restore_command = 'cp "+postgresWALArchivePath) {
		t.Errorf("conf = %q, missing restore_command against %q", conf, postgresWALArchivePath)
	}
	if !strings.Contains(conf, "recovery_target_time = '2026-08-14 06:30:15.123456+00'") {
		t.Errorf("conf = %q, want recovery_target_time for 2026-08-14 06:30:15.123456+00", conf)
	}
	if !strings.Contains(conf, "recovery_target_action = 'promote'") {
		t.Errorf("conf = %q, missing recovery_target_action = promote", conf)
	}

	cmd := strings.Join(call.cmd, " ")
	if !strings.Contains(cmd, "touch") || !strings.Contains(cmd, "recovery.signal") {
		t.Errorf("command = %q, missing recovery.signal touch", cmd)
	}
	if !strings.Contains(cmd, "include '"+pitrConfFile+"'") {
		t.Errorf("command = %q, missing postgresql.auto.conf include line", cmd)
	}
}

func TestContainerPITRRestorer_Restore_ExtractFailure_StillRemovesHelper(t *testing.T) {
	rt := &recordingHelperRuntime{execInputErr: errors.New("extract failed")}
	r := &ContainerPITRRestorer{Runtime: rt}

	err := r.Restore(context.Background(), "db-mydb-data", strings.NewReader("tar-bytes"), time.Now())
	if err == nil {
		t.Fatal("Restore() error = nil, want the extract failure")
	}
	if rt.removeCalls != 1 {
		t.Errorf("removeCalls = %d, want 1 even on failure (helper must never be leaked)", rt.removeCalls)
	}
}
