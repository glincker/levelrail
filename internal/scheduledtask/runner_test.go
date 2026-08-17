package scheduledtask

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeRunnerAppStore is Runner's own fake AppStore, the same shape
// exec_test.go's fakeExecAppRuntime establishes for this codebase's
// hand-written fakes: a struct literal a test configures directly, no
// mocking framework.
type fakeRunnerAppStore struct {
	svc *store.DesiredService
	err error
}

func (f *fakeRunnerAppStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return f.svc, f.err
}

type finishedCall struct {
	id, status, output, errMsg, finishedAt string
	exitCode                               int
}

// fakeRunnerHistoryStore is Runner's own fake HistoryStore: records every
// Start/Finish call, and can be told to fail either.
type fakeRunnerHistoryStore struct {
	mu sync.Mutex

	started  []store.ScheduledTaskRun
	startErr error

	finished  []finishedCall
	finishErr error
}

func (f *fakeRunnerHistoryStore) StartScheduledTaskRun(_ context.Context, r store.ScheduledTaskRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, r)
	return f.startErr
}

func (f *fakeRunnerHistoryStore) FinishScheduledTaskRun(_ context.Context, id, status string, exitCode int, output, errMsg, finishedAt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, finishedCall{id: id, status: status, exitCode: exitCode, output: output, errMsg: errMsg, finishedAt: finishedAt})
	return f.finishErr
}

// fakeRunnerRuntime is a hand-written fake docker.Runtime, the same
// convention internal/api/exec_test.go's fakeExecAppRuntime already
// establishes for this exact interface: every method is a
// compile-satisfying stub except the two Runner actually calls
// (InspectByName, Exec), which are configurable per test.
type fakeRunnerRuntime struct {
	inspectState *docker.ContainerState
	inspectErr   error

	execReader io.ReadCloser
	execErr    error

	gotContainerID string
	gotCmd         []string
	execCalls      int
}

func (f *fakeRunnerRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return f.inspectState, f.inspectErr
}

func (f *fakeRunnerRuntime) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	f.execCalls++
	f.gotContainerID = containerID
	f.gotCmd = cmd
	return f.execReader, f.execErr
}

func (f *fakeRunnerRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeRunnerRuntime) Create(context.Context, docker.ContainerSpec) (string, error) { return "", nil }
func (f *fakeRunnerRuntime) Start(context.Context, string) error                          { return nil }
func (f *fakeRunnerRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeRunnerRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeRunnerRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeRunnerRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeRunnerRuntime) Remove(context.Context, string, bool) error       { return nil }
func (f *fakeRunnerRuntime) EnsureVolume(context.Context, string) error       { return nil }
func (f *fakeRunnerRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeRunnerRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeRunnerRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

var _ docker.Runtime = (*fakeRunnerRuntime)(nil)

// errReadCloser reads r to completion and then returns err instead of
// io.EOF, the same "some bytes, then a trailing non-EOF error" shape
// docker.Client.Exec's real returned io.ReadCloser has once a command
// exits non-zero (exec_test.go's own identical type).
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

func testTask() store.ScheduledTask {
	return store.ScheduledTask{ID: "sct_1", ServiceName: "web", Name: "cron", Command: "php artisan schedule:run", Schedule: "* * * * *"}
}

func newTestRunner(app *fakeRunnerAppStore, rt *fakeRunnerRuntime, hist *fakeRunnerHistoryStore) *Runner {
	return &Runner{
		Apps:    app,
		Runtime: func(string) (docker.Runtime, error) { return rt, nil },
		History: hist,
	}
}

func TestRunner_Run_Success(t *testing.T) {
	rt := &fakeRunnerRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("done\n")),
	}
	app := &fakeRunnerAppStore{svc: &store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	hist := &fakeRunnerHistoryStore{}
	r := newTestRunner(app, rt, hist)

	if err := r.Run(context.Background(), "sctr_1", testTask()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(hist.started) != 1 || hist.started[0].ID != "sctr_1" || hist.started[0].ScheduledTaskID != "sct_1" {
		t.Fatalf("started = %+v, want one row for sctr_1/sct_1", hist.started)
	}
	if len(hist.finished) != 1 {
		t.Fatalf("finished calls = %d, want 1", len(hist.finished))
	}
	got := hist.finished[0]
	if got.status != store.ScheduledTaskRunStatusSucceeded {
		t.Errorf("status = %q, want %q", got.status, store.ScheduledTaskRunStatusSucceeded)
	}
	if got.output != "done\n" {
		t.Errorf("output = %q, want %q", got.output, "done\n")
	}
	if got.exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", got.exitCode)
	}

	wantCmd := []string{"sh", "-c", "php artisan schedule:run"}
	if len(rt.gotCmd) != len(wantCmd) {
		t.Fatalf("Exec cmd = %v, want %v", rt.gotCmd, wantCmd)
	}
	for i := range wantCmd {
		if rt.gotCmd[i] != wantCmd[i] {
			t.Errorf("Exec cmd = %v, want %v", rt.gotCmd, wantCmd)
			break
		}
	}
	if rt.gotContainerID != "c1" {
		t.Errorf("Exec container = %q, want c1", rt.gotContainerID)
	}
}

// TestRunner_Run_NonzeroExit proves a command that runs but fails inside
// the container is recorded as a failed run with the real exit code, and
// Run itself returns that as an error: the scheduler-facing counterpart
// of exec.go's own "still a 200, not a transport failure" contract.
func TestRunner_Run_NonzeroExit(t *testing.T) {
	execErr := &docker.ExecExitError{Cmd: []string{"sh", "-c", "false"}, Container: "c1", ExitCode: 7, Stderr: "boom"}
	rt := &fakeRunnerRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader("partial\n"), err: execErr},
	}
	app := &fakeRunnerAppStore{svc: &store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	hist := &fakeRunnerHistoryStore{}
	r := newTestRunner(app, rt, hist)

	task := testTask()
	task.Command = "false"
	if err := r.Run(context.Background(), "sctr_1", task); err == nil {
		t.Fatal("Run() error = nil, want a nonzero-exit error")
	}

	if len(hist.finished) != 1 {
		t.Fatalf("finished calls = %d, want 1", len(hist.finished))
	}
	got := hist.finished[0]
	if got.status != store.ScheduledTaskRunStatusFailed {
		t.Errorf("status = %q, want %q", got.status, store.ScheduledTaskRunStatusFailed)
	}
	if got.exitCode != 7 {
		t.Errorf("exitCode = %d, want 7", got.exitCode)
	}
	if !strings.Contains(got.output, "partial") || !strings.Contains(got.output, "boom") {
		t.Errorf("output = %q, want it to contain both stdout and stderr", got.output)
	}
}

// TestRunner_Run_NoRunningContainer covers the same "app has no running
// container" case exec.go's own 409 handles, here recorded as a failed
// run instead of an HTTP status.
func TestRunner_Run_NoRunningContainer(t *testing.T) {
	rt := &fakeRunnerRuntime{inspectState: nil}
	app := &fakeRunnerAppStore{svc: &store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	hist := &fakeRunnerHistoryStore{}
	r := newTestRunner(app, rt, hist)

	if err := r.Run(context.Background(), "sctr_1", testTask()); err == nil {
		t.Fatal("Run() error = nil, want an error for no running container")
	}
	if rt.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0: no running container to exec into", rt.execCalls)
	}
	if len(hist.finished) != 1 || hist.finished[0].status != store.ScheduledTaskRunStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hist.finished)
	}
}

// TestRunner_Run_AppLoadFailure covers a task whose owning service can no
// longer be resolved (e.g. deleted out from under a task the CASCADE
// hadn't yet caught up with).
func TestRunner_Run_AppLoadFailure(t *testing.T) {
	app := &fakeRunnerAppStore{err: errors.New("service not found")}
	rt := &fakeRunnerRuntime{}
	hist := &fakeRunnerHistoryStore{}
	r := newTestRunner(app, rt, hist)

	if err := r.Run(context.Background(), "sctr_1", testTask()); err == nil {
		t.Fatal("Run() error = nil, want an error when the app can't be resolved")
	}
	if rt.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0", rt.execCalls)
	}
	if len(hist.finished) != 1 || hist.finished[0].status != store.ScheduledTaskRunStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hist.finished)
	}
}

// TestRunner_Run_Timeout proves a command whose output stream never
// resolves does not hold Run open past the task's own timeout.
func TestRunner_Run_Timeout(t *testing.T) {
	blocking := &blockingReadCloser{closed: make(chan struct{})}
	t.Cleanup(func() { _ = blocking.Close() })
	rt := &fakeRunnerRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   blocking,
	}
	app := &fakeRunnerAppStore{svc: &store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	hist := &fakeRunnerHistoryStore{}
	r := newTestRunner(app, rt, hist)

	task := testTask()
	task.TimeoutSeconds = 1

	start := time.Now()
	err := r.Run(context.Background(), "sctr_1", task)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() error = nil, want a timeout error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Run() took %s, want well under the requested 1s timeout", elapsed)
	}
	if len(hist.finished) != 1 || hist.finished[0].status != store.ScheduledTaskRunStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hist.finished)
	}
}

type blockingReadCloser struct {
	closed chan struct{}
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

// TestRunner_Run_StartHistoryFailure proves a failure to even write the
// initial "running" row aborts before ever touching the container, and
// is returned as-is rather than swallowed.
func TestRunner_Run_StartHistoryFailure(t *testing.T) {
	rt := &fakeRunnerRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	app := &fakeRunnerAppStore{svc: &store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	hist := &fakeRunnerHistoryStore{startErr: errors.New("db unavailable")}
	r := newTestRunner(app, rt, hist)

	if err := r.Run(context.Background(), "sctr_1", testTask()); err == nil {
		t.Fatal("Run() error = nil, want the start-history failure")
	}
	if rt.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0: must never run once StartScheduledTaskRun fails", rt.execCalls)
	}
	if len(hist.finished) != 0 {
		t.Errorf("finished calls = %d, want 0", len(hist.finished))
	}
}
