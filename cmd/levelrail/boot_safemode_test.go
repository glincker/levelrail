package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/safemode"
)

func quietBootLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPrepareBootCleanDataDir(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	_, _, entry := prepareBoot(context.Background(), quietBootLogger())
	if entry != nil {
		t.Fatalf("a fresh data dir must boot normally, got %+v", entry)
	}
}

func TestPrepareBootCorruptDatabaseEntersSafeMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, storeFilename), []byte("this is not a sqlite file, just text padding"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, entry := prepareBoot(context.Background(), quietBootLogger())
	if entry == nil || entry.Reason != safemode.ReasonDBCorrupt {
		t.Fatalf("want db_corrupt safe mode, got %+v", entry)
	}
}

// TestPrepareBootCrashLoopStopsRestarting simulates a supervisor restarting a process that never becomes healthy.
func TestPrepareBootCrashLoopStopsRestarting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	t.Setenv(safemode.EnvBackoffBase, "1ms")
	t.Setenv(safemode.EnvBackoffMax, "4ms")
	ctx := context.Background()

	var entries []*safemode.Entry
	for boot := 0; boot < 5; boot++ {
		ledger, _, entry := prepareBoot(ctx, quietBootLogger())
		entries = append(entries, entry)
		if entry == nil {
			ledger.NoteError(errors.New("docker unreachable"))
		}
	}
	for i, want := range []bool{false, false, false, true, true} {
		if (entries[i] != nil) != want {
			t.Fatalf("boot %d: safe mode = %v, want %v", i, entries[i] != nil, want)
		}
	}
	if e := entries[3]; e.Reason != safemode.ReasonCrashLoop || e.Detail != "docker unreachable" {
		t.Fatalf("crash loop entry should carry the last error: %+v", e)
	}
}

func TestPrepareBootBackoffWaitsAndHonoursCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dir)
	t.Setenv(safemode.EnvBackoffBase, "300ms")
	safemode.NewLedger(dir).Begin()

	var waits []time.Duration
	orig := bootWait
	bootWait = func(_ context.Context, d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() { bootWait = orig })

	prepareBoot(context.Background(), quietBootLogger())
	if len(waits) != 1 || waits[0] <= 0 {
		t.Fatalf("a boot after a failed one must wait out the backoff, got %v", waits)
	}
}

func TestBootWaitHonoursCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { bootWait(ctx, time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a stop signal during the backoff must end the wait")
	}
}

func TestClassifyStoreError(t *testing.T) {
	cases := map[string]error{
		safemode.ReasonMigrationFailed: errors.New("store: migrate /x/levelrail.db: apply 0042: boom"),
		safemode.ReasonStoreFailed:     errors.New("store: open /x/levelrail.db: unable to open database file"),
	}
	for want, err := range cases {
		if got := classifyStoreError(err, 2); got.entry.Reason != want || got.entry.Failures != 2 {
			t.Fatalf("%v classified as %+v, want %s", err, got.entry, want)
		}
	}
}
