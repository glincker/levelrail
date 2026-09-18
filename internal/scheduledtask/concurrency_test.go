package scheduledtask

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestRunner_BeginRun_Allow_AlwaysProceedsIndependently proves the
// allow policy never skips or cancels anything: two overlapping
// beginRun calls for the same task ID both proceed, and each one's
// cleanup only ever removes its own entry, never the other's.
func TestRunner_BeginRun_Allow_AlwaysProceedsIndependently(t *testing.T) {
	r := &Runner{}
	ctx := context.Background()

	_, cleanup1, skip1 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyAllow)
	if skip1 {
		t.Fatal("first beginRun(allow) skipped, want it to always proceed")
	}
	_, cleanup2, skip2 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyAllow)
	if skip2 {
		t.Fatal("second beginRun(allow) skipped, want it to proceed alongside the first")
	}

	cleanup1()
	r.mu.Lock()
	_, stillTracked := r.inFlight["t1"]
	r.mu.Unlock()
	if !stillTracked {
		t.Error("cleanup1 removed the second run's own tracked entry, want it untouched")
	}

	cleanup2()
	r.mu.Lock()
	_, stillTracked = r.inFlight["t1"]
	r.mu.Unlock()
	if stillTracked {
		t.Error("task still tracked as in flight after both cleanups ran")
	}
}

// TestRunner_BeginRun_Forbid_SkipsWhileInFlight proves forbid skips a
// second beginRun for the same task ID while the first is still
// registered, then allows a fresh run through once the first cleans up.
func TestRunner_BeginRun_Forbid_SkipsWhileInFlight(t *testing.T) {
	r := &Runner{}
	ctx := context.Background()

	_, cleanup, skip := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyForbid)
	if skip {
		t.Fatal("first beginRun(forbid) skipped, want nothing in flight yet")
	}

	if _, _, skip2 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyForbid); !skip2 {
		t.Error("second beginRun(forbid) proceeded, want it to skip while the first is in flight")
	}

	cleanup()

	_, cleanup3, skip3 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyForbid)
	if skip3 {
		t.Error("beginRun(forbid) skipped after the previous run's cleanup, want nothing in flight anymore")
	} else {
		cleanup3()
	}
}

// TestRunner_BeginRun_Replace_CancelsPreviousAndWaits proves replace
// cancels the previous run's context with errReplacedByNewRun and blocks
// until that run's own cleanup has actually run, so the two runs'
// RecordScheduledTaskRun calls can never land out of order.
func TestRunner_BeginRun_Replace_CancelsPreviousAndWaits(t *testing.T) {
	r := &Runner{}
	ctx := context.Background()

	runCtx1, cleanup1, skip1 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyReplace)
	if skip1 {
		t.Fatal("first beginRun(replace) skipped, want nothing in flight yet")
	}

	cleanupRan := make(chan struct{})
	go func() {
		<-runCtx1.Done()
		if !isReplaced(runCtx1) {
			t.Error("runCtx1 was cancelled but not by a replace takeover")
		}
		cleanup1()
		close(cleanupRan)
	}()

	start := time.Now()
	_, cleanup2, skip2 := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyReplace)
	if skip2 {
		t.Fatal("second beginRun(replace) skipped, want it to cancel the first and proceed")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("beginRun(replace) took %s, want it to return promptly once the first cleaned up", elapsed)
	}

	select {
	case <-cleanupRan:
	default:
		t.Error("beginRun(replace) returned before the replaced run's own cleanup ran")
	}
	cleanup2()
}

// TestRunner_Run_ConcurrencyForbid_SkipsWhileInFlight is the end-to-end
// counterpart to TestRunner_BeginRun_Forbid_SkipsWhileInFlight: a real
// Run call blocked mid-exec, and a second Run call for the same task
// that must be skipped and recorded as ScheduledTaskStatusSkippedConcurrency
// rather than touching the container at all.
func TestRunner_Run_ConcurrencyForbid_SkipsWhileInFlight(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	blocking := newBlockingReadCloser()
	t.Cleanup(func() { _ = blocking.Close() })
	rt := &fakeRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   blocking,
		execStarted:  make(chan struct{}, 1),
	}
	r := newTestRunner(apps, runs, rt, nil)

	task := testTask()
	task.ConcurrencyPolicy = store.ScheduledTaskConcurrencyForbid

	firstDone := make(chan error, 1)
	go func() { firstDone <- r.Run(context.Background(), task) }()

	select {
	case <-rt.execStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first run to reach its exec phase")
	}

	if err := r.Run(context.Background(), task); err != nil {
		t.Fatalf("second Run() (forbid, should skip) error = %v", err)
	}

	_ = blocking.Close()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first run to finish after closing its exec stream")
	}

	calls := runs.snapshot()
	if len(calls) != 2 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 2 (one skip, one real outcome)", len(calls))
	}
	if calls[0].status != store.ScheduledTaskStatusSkippedConcurrency {
		t.Errorf("first recorded call status = %q, want %q (the second Run call's skip happens first, while the first is still blocked)", calls[0].status, store.ScheduledTaskStatusSkippedConcurrency)
	}
	if calls[1].status != store.ScheduledTaskStatusSuccess {
		t.Errorf("second recorded call status = %q, want %q (the originally-blocked run, once unblocked)", calls[1].status, store.ScheduledTaskStatusSuccess)
	}
}

// TestRunner_Run_ConcurrencyReplace_CancelsInFlightRun proves a replace
// policy cancels a still-running invocation (recorded as
// ScheduledTaskStatusReplaced) before the newer invocation proceeds to
// its own exec and success outcome.
func TestRunner_Run_ConcurrencyReplace_CancelsInFlightRun(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	blocking := newBlockingReadCloser()
	t.Cleanup(func() { _ = blocking.Close() })
	rt := &fakeRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   blocking,
		execStarted:  make(chan struct{}, 1),
	}
	r := newTestRunner(apps, runs, rt, nil)

	task := testTask()
	task.ConcurrencyPolicy = store.ScheduledTaskConcurrencyReplace

	firstDone := make(chan error, 1)
	go func() { firstDone <- r.Run(context.Background(), task) }()

	select {
	case <-rt.execStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first run to reach its exec phase")
	}

	// The first run's rc.Close() (triggered by its own cancellation)
	// closes the shared blockingReadCloser, so the second run's Exec
	// call (returning the same fake reader) reads an immediate EOF: a
	// deliberate fake-runtime simplification, not something the second
	// run needs to know about.
	if err := r.Run(context.Background(), task); err != nil {
		t.Fatalf("second Run() (replace) error = %v", err)
	}

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the replaced run to finish")
	}

	calls := runs.snapshot()
	if len(calls) != 2 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 2 (one replaced, one real outcome)", len(calls))
	}
	if calls[0].status != store.ScheduledTaskStatusReplaced {
		t.Errorf("first recorded call status = %q, want %q", calls[0].status, store.ScheduledTaskStatusReplaced)
	}
	if calls[1].status != store.ScheduledTaskStatusSuccess {
		t.Errorf("second recorded call status = %q, want %q", calls[1].status, store.ScheduledTaskStatusSuccess)
	}
}

// TestRunner_Run_ConcurrencyAllow_BothRunsRecordIndependently proves the
// default allow policy still lets two overlapping runs both proceed to
// completion, neither one skipped nor cancelled: today's existing
// behavior, now exercised explicitly against the concurrency machinery
// rather than only implicitly.
func TestRunner_Run_ConcurrencyAllow_BothRunsRecordIndependently(t *testing.T) {
	apps := &fakeAppStore{svc: store.DesiredService{Name: "web", Image: "levelrail/web:1"}}
	runs := &fakeRunStore{}
	rt := &fakeRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("hi\n")),
	}
	r := newTestRunner(apps, runs, rt, nil)

	task := testTask()
	task.ConcurrencyPolicy = store.ScheduledTaskConcurrencyAllow

	if err := r.Run(context.Background(), task); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := r.Run(context.Background(), task); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	calls := runs.snapshot()
	if len(calls) != 2 {
		t.Fatalf("RecordScheduledTaskRun calls = %d, want 2", len(calls))
	}
	for i, c := range calls {
		if c.status != store.ScheduledTaskStatusSuccess {
			t.Errorf("call %d status = %q, want %q", i, c.status, store.ScheduledTaskStatusSuccess)
		}
	}
}
