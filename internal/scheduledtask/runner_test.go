package scheduledtask

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAppStore is Runner's own fake for AppStore.
type fakeAppStore struct {
	svc store.DesiredService
	err error
}

func (f *fakeAppStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.svc, nil
}

// fakeRunStore is Runner's own fake for RunStore: records every call.
type fakeRunStore struct {
	calls []struct {
		id     string
		ranAt  time.Time
		status string
		output string
	}
	err error
}

func (f *fakeRunStore) RecordScheduledTaskRun(_ context.Context, id string, ranAt time.Time, status, output string) error {
	f.calls = append(f.calls, struct {
		id     string
		ranAt  time.Time
		status string
		output string
	}{id, ranAt, status, output})
	return f.err
}

// fakeRuntime is a hand-written fake docker.Runtime, the same convention
// internal/api/exec_test.go's own fakeExecAppRuntime establishes for
// this exact interface: every method is a compile-satisfying stub except
// InspectByName/Exec, which are configurable per test.
type fakeRuntime struct {
	inspectState *docker.ContainerState
	inspectErr   error
	execReader   io.ReadCloser
	execErr      error

	gotContainerID string
	gotCmd         []string
}

func (f *fakeRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return f.inspectState, f.inspectErr
}
func (f *fakeRuntime) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	f.gotContainerID = containerID
	f.gotCmd = cmd
	return f.execReader, f.execErr
}
func (f *fakeRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeRuntime) Create(context.Context, docker.ContainerSpec) (string, error) { return "", nil }
func (f *fakeRuntime) Start(context.Context, string) error                          { return nil }
func (f *fakeRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
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
func (f *fakeRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakeRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

var _ docker.Runtime = (*fakeRuntime)(nil)

func newTestRunner(apps *fakeAppStore, runs *fakeRunStore, rt docker.Runtime, resolveErr error) *Runner {
	return &Runner{
		Apps: apps,
		Runs: runs,
		Resolver: func(string) (docker.Runtime, error) {
			if resolveErr != nil {
				return nil, resolveErr
			}
			return rt, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) },
	}
}

func testTask() store.ScheduledTask {
	return store.ScheduledTask{ID: "st_1", ServiceName: "web", Command: []string{"sh", "-c", "echo hi"}, Schedule: "0 3 * * *", Enabled: true}
}

func TestRunner_Run_Success(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	rt := &fakeRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("hello\n")),
	}
	r := newTestRunner(apps, runs, rt, nil)

	if err := r.Run(context.Background(), testTask()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(runs.calls) != 1 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 1", len(runs.calls))
	}
	got := runs.calls[0]
	if got.status != store.ScheduledTaskStatusSuccess {
		t.Errorf("status = %q, want %q", got.status, store.ScheduledTaskStatusSuccess)
	}
	if got.output != "hello\n" {
		t.Errorf("output = %q, want %q", got.output, "hello\n")
	}
	if rt.gotContainerID != "c1" {
		t.Errorf("Exec called with container %q, want c1", rt.gotContainerID)
	}
	wantCmd := []string{"sh", "-c", "echo hi"}
	if len(rt.gotCmd) != len(wantCmd) {
		t.Fatalf("Exec cmd = %v, want %v", rt.gotCmd, wantCmd)
	}
}

// errReadCloser reads r to completion then returns err instead of
// io.EOF, the same shape docker.Client.Exec's real ReadCloser has once a
// command exits non-zero (mirrors internal/api/exec_test.go's own
// errReadCloser).
type errReadCloser struct {
	r   io.Reader
	err error
}

func (e *errReadCloser) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		return n, e.err
	}
	return n, err
}
func (e *errReadCloser) Close() error { return nil }

func TestRunner_Run_NonzeroExit_RecordsFailedWithStderr(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	execErr := &docker.ExecExitError{Cmd: []string{"false"}, Container: "c1", ExitCode: 1, Stderr: "boom"}
	rt := &fakeRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader("partial\n"), err: execErr},
	}
	r := newTestRunner(apps, runs, rt, nil)

	if err := r.Run(context.Background(), testTask()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(runs.calls) != 1 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 1", len(runs.calls))
	}
	got := runs.calls[0]
	if got.status != store.ScheduledTaskStatusFailed {
		t.Errorf("status = %q, want %q", got.status, store.ScheduledTaskStatusFailed)
	}
	if !strings.Contains(got.output, "partial") || !strings.Contains(got.output, "boom") {
		t.Errorf("output = %q, want it to contain both stdout and stderr", got.output)
	}
}

func TestRunner_Run_ContainerNotRunning(t *testing.T) {
	tests := []struct {
		name  string
		state *docker.ContainerState
	}{
		{name: "never deployed", state: nil},
		{name: "exists but stopped", state: &docker.ContainerState{ID: "c1", Running: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
			runs := &fakeRunStore{}
			rt := &fakeRuntime{inspectState: tc.state}
			r := newTestRunner(apps, runs, rt, nil)

			if err := r.Run(context.Background(), testTask()); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if len(runs.calls) != 1 {
				t.Fatalf("RecordScheduledTaskRun calls = %d, want 1", len(runs.calls))
			}
			if got := runs.calls[0].status; got != store.ScheduledTaskStatusContainerNotRunning {
				t.Errorf("status = %q, want %q", got, store.ScheduledTaskStatusContainerNotRunning)
			}
			if rt.gotCmd != nil {
				t.Error("Exec called, want it never called when the container isn't running")
			}
		})
	}
}

// TestRunner_Run_NodeUnreachable proves a NodeRuntimeResolver failure
// (the app's node isn't currently connected) is recorded as a clear
// status on the task's own row, not returned as an error that would
// look like a scheduler-crashing infrastructure failure: see Runner.Run's
// own doc comment.
func TestRunner_Run_NodeUnreachable(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	r := newTestRunner(apps, runs, nil, errors.New("node not registered"))

	if err := r.Run(context.Background(), testTask()); err != nil {
		t.Fatalf("Run() error = %v, want nil (recorded, not returned)", err)
	}
	if len(runs.calls) != 1 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 1", len(runs.calls))
	}
	if got := runs.calls[0].status; got != store.ScheduledTaskStatusContainerNotRunning {
		t.Errorf("status = %q, want %q", got, store.ScheduledTaskStatusContainerNotRunning)
	}
}

// blockingReadCloser never returns from Read until Close is called,
// mirroring internal/api/exec_test.go's own helper of the same name,
// simulating a command whose output stream never resolves on its own.
type blockingReadCloser struct {
	closed chan struct{}
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{closed: make(chan struct{})}
}
func (b *blockingReadCloser) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}
func (b *blockingReadCloser) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func TestRunner_Run_Timeout(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	blocking := newBlockingReadCloser()
	t.Cleanup(func() { _ = blocking.Close() })
	rt := &fakeRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}, execReader: blocking}
	r := newTestRunner(apps, runs, rt, nil)
	r.Timeout = 50 * time.Millisecond

	start := time.Now()
	if err := r.Run(context.Background(), testTask()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run() took %s, want well under its own 50ms timeout", elapsed)
	}
	if len(runs.calls) != 1 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 1", len(runs.calls))
	}
	if got := runs.calls[0].status; got != store.ScheduledTaskStatusTimeout {
		t.Errorf("status = %q, want %q", got, store.ScheduledTaskStatusTimeout)
	}
}

func TestRunner_Run_LoadServiceFailed_ReturnsError(t *testing.T) {
	apps := &fakeAppStore{err: errors.New("db down")}
	runs := &fakeRunStore{}
	r := newTestRunner(apps, runs, &fakeRuntime{}, nil)

	err := r.Run(context.Background(), testTask())
	if err == nil {
		t.Fatal("Run() error = nil, want a genuine infrastructure error")
	}
	if len(runs.calls) != 0 {
		t.Errorf("RecordScheduledTaskRun calls = %d, want 0: never got far enough to run anything", len(runs.calls))
	}
}

// TestTailCappedWriter_KeepsMostRecentBytes proves the tail-not-head
// truncation behavior maxOutputBytes's own doc comment promises.
func TestTailCappedWriter_KeepsMostRecentBytes(t *testing.T) {
	w := newTailCappedWriter(10)
	_, _ = w.Write([]byte("0123456789"))
	_, _ = w.Write([]byte("ABCDEFGHIJ"))

	got := w.String()
	if !strings.HasSuffix(got, "ABCDEFGHIJ") {
		t.Errorf("String() = %q, want it to end with the most recently written bytes", got)
	}
	if !w.truncated() {
		t.Error("truncated() = false, want true")
	}
}
