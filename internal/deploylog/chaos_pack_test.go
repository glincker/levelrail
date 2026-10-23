package deploylog

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

// Disk full while persisting build logs: live viewers still get lines,
// nothing partial is stored, and the next attempt persists once space returns.
func TestChaosPack_DiskFullOnLogWrite_LiveFeedIntactAndNextAttemptPersists(t *testing.T) {
	store := &fakeStore{err: errors.New("write deploy_logs: no space left on device")}
	r := NewRecorder(store, discardLogger())

	r.Start("dep_1")
	lines, live, unsub, ok := r.Snapshot("dep_1")
	if !ok || len(lines) != 0 {
		t.Fatalf("Snapshot() = %v, %v, want ok and empty", lines, ok)
	}
	defer unsub()
	progress := r.Progress("dep_1")
	for i := 0; i < batchMaxLines+5; i++ {
		progress(build.ProgressEvent{Log: "line", Stream: "stdout"})
	}
	if again, _, unsub2, ok := r.Snapshot("dep_1"); !ok || len(again) != batchMaxLines+5 {
		t.Fatalf("in-memory lines = %d (ok=%v), want %d: a full disk must not drop the live tail", len(again), ok, batchMaxLines+5)
	} else {
		unsub2()
	}
	r.Finish(context.Background(), "dep_1")

	got := 0
	for range live {
		got++
	}
	if got == 0 {
		t.Fatal("live subscriber received nothing while the store was full")
	}
	if n := len(store.snapshot()); n != 0 {
		t.Fatalf("store holds %d entries from failed writes, want none", n)
	}

	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	r.Start("dep_2")
	r.Progress("dep_2")(build.ProgressEvent{Log: "after recovery", Stream: "stdout"})
	r.Finish(context.Background(), "dep_2")
	written := store.snapshot()
	if len(written) != 1 || written[0].AttemptID != "dep_2" || written[0].Message != "after recovery" {
		t.Fatalf("persisted = %+v, want only dep_2's line", written)
	}
}
