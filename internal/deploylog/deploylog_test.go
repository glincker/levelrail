package deploylog

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// fakeStore is a hand-written fake for LogStore, matching this
// codebase's "no mocking framework" convention.
type fakeStore struct {
	mu      sync.Mutex
	written []telemetry.DeployLogEntry
	batches int
	err     error
}

func (f *fakeStore) WriteDeployLogBatch(_ context.Context, entries []telemetry.DeployLogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.batches++
	f.written = append(f.written, entries...)
	return nil
}

func (f *fakeStore) snapshot() []telemetry.DeployLogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]telemetry.DeployLogEntry, len(f.written))
	copy(out, f.written)
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestRecorder_ProgressPersistsLogLines(t *testing.T) {
	store := &fakeStore{}
	r := NewRecorder(store, discardLogger())

	r.Start("dep_1")
	progress := r.Progress("dep_1")
	progress(build.ProgressEvent{Step: "step1", Completed: true}) // lifecycle event, no Log: must not be persisted
	progress(build.ProgressEvent{Log: "line one", Stream: "stdout"})
	progress(build.ProgressEvent{Log: "line two", Stream: "stderr"})
	r.Finish(context.Background(), "dep_1")

	got := store.snapshot()
	if len(got) != 2 {
		t.Fatalf("got %d persisted lines, want 2", len(got))
	}
	if got[0].Message != "line one" || got[0].Stream != "stdout" || got[0].AttemptID != "dep_1" {
		t.Errorf("line 0 = %+v", got[0])
	}
	if got[1].Message != "line two" || got[1].Stream != "stderr" {
		t.Errorf("line 1 = %+v", got[1])
	}
}

func TestRecorder_ProgressDefaultsEmptyStreamToStdout(t *testing.T) {
	store := &fakeStore{}
	r := NewRecorder(store, discardLogger())
	r.Start("dep_1")
	r.Progress("dep_1")(build.ProgressEvent{Log: "some output"}) // no Stream set
	r.Finish(context.Background(), "dep_1")

	got := store.snapshot()
	if len(got) != 1 || got[0].Stream != "stdout" {
		t.Fatalf("got %+v, want one stdout line", got)
	}
}

func TestRecorder_BatchesAtThreshold(t *testing.T) {
	store := &fakeStore{}
	r := NewRecorder(store, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")

	for i := 0; i < batchMaxLines; i++ {
		progress(build.ProgressEvent{Log: "line", Stream: "stdout"})
	}

	// The batch should have flushed on its own once the threshold was
	// hit, before Finish is ever called.
	store.mu.Lock()
	batchesBeforeFinish := store.batches
	writtenBeforeFinish := len(store.written)
	store.mu.Unlock()
	if batchesBeforeFinish == 0 {
		t.Error("expected at least one batch flush before reaching batchMaxLines threshold's caller, Finish")
	}
	if writtenBeforeFinish != batchMaxLines {
		t.Errorf("written = %d, want %d", writtenBeforeFinish, batchMaxLines)
	}

	r.Finish(context.Background(), "dep_1")
	if len(store.snapshot()) != batchMaxLines {
		t.Errorf("after Finish, written = %d, want %d (no double-write)", len(store.snapshot()), batchMaxLines)
	}
}

func TestRecorder_SnapshotBeforeStartReturnsNotOK(t *testing.T) {
	r := NewRecorder(&fakeStore{}, discardLogger())
	_, _, _, ok := r.Snapshot("dep_never-started")
	if ok {
		t.Error("Snapshot() ok = true for an attempt never Start()ed, want false")
	}
}

func TestRecorder_SnapshotAfterFinishReturnsNotOK(t *testing.T) {
	r := NewRecorder(&fakeStore{}, discardLogger())
	r.Start("dep_1")
	r.Finish(context.Background(), "dep_1")
	_, _, _, ok := r.Snapshot("dep_1")
	if ok {
		t.Error("Snapshot() ok = true after Finish, want false: caller should fall back to the persisted store")
	}
}

func TestRecorder_SnapshotReturnsLinesSoFarAndLiveEvents(t *testing.T) {
	r := NewRecorder(&fakeStore{}, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")
	progress(build.ProgressEvent{Log: "before subscribe", Stream: "stdout"})

	lines, live, unsubscribe, ok := r.Snapshot("dep_1")
	defer unsubscribe()
	if !ok {
		t.Fatal("Snapshot() ok = false, want true for an active attempt")
	}
	if len(lines) != 1 || lines[0].Line != "before subscribe" {
		t.Fatalf("lines = %+v, want one line 'before subscribe'", lines)
	}

	progress(build.ProgressEvent{Log: "after subscribe", Stream: "stderr"})

	select {
	case ev := <-live:
		if ev.Line != "after subscribe" || ev.Stream != "stderr" {
			t.Errorf("live event = %+v, want {after subscribe stderr}", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a live event published after Snapshot")
	}

	r.Finish(context.Background(), "dep_1")

	select {
	case _, open := <-live:
		if open {
			t.Error("expected the live channel to be closed (empty read with open=false) after Finish")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the live channel to close after Finish")
	}
}

func TestRecorder_UnsubscribeStopsDelivery(t *testing.T) {
	r := NewRecorder(&fakeStore{}, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")

	_, live, unsubscribe, ok := r.Snapshot("dep_1")
	if !ok {
		t.Fatal("Snapshot() ok = false")
	}
	unsubscribe()

	progress(build.ProgressEvent{Log: "after unsubscribe", Stream: "stdout"})

	select {
	case ev, open := <-live:
		if open {
			t.Errorf("received event %+v on an unsubscribed channel, want no delivery", ev)
		}
		// A closed channel read (open=false) is also acceptable here if
		// Finish races in; the real assertion is "no event value ever
		// arrives", covered by the default case below actually firing
		// first in practice.
	case <-time.After(200 * time.Millisecond):
		// No delivery within a reasonable window: the expected outcome.
	}

	r.Finish(context.Background(), "dep_1")
}

func TestRecorder_FinishFlushesRemainder(t *testing.T) {
	store := &fakeStore{}
	r := NewRecorder(store, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")
	progress(build.ProgressEvent{Log: "only line", Stream: "stdout"})

	if len(store.snapshot()) != 0 {
		t.Fatalf("expected nothing flushed yet before threshold or Finish, got %d", len(store.snapshot()))
	}

	r.Finish(context.Background(), "dep_1")
	got := store.snapshot()
	if len(got) != 1 || got[0].Message != "only line" {
		t.Fatalf("got %+v, want the buffered line flushed by Finish", got)
	}
}

func TestRecorder_FinishOnUnknownAttemptIsNoop(_ *testing.T) {
	r := NewRecorder(&fakeStore{}, discardLogger())
	r.Finish(context.Background(), "dep_never-started") // must not panic
}

func TestRecorder_StoreErrorIsLoggedNotPanicked(_ *testing.T) {
	store := &fakeStore{err: errors.New("disk full")}
	r := NewRecorder(store, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")
	progress(build.ProgressEvent{Log: "line", Stream: "stdout"})
	r.Finish(context.Background(), "dep_1") // must not panic despite the store failing
}

func TestRecorder_NilStoreIsTolerated(_ *testing.T) {
	r := NewRecorder(nil, discardLogger())
	r.Start("dep_1")
	progress := r.Progress("dep_1")
	progress(build.ProgressEvent{Log: "line", Stream: "stdout"})
	r.Finish(context.Background(), "dep_1") // must not panic with no store configured

	lines, _, unsubscribe, ok := r.Snapshot("dep_1")
	if ok {
		unsubscribe()
	}
	_ = lines
}
