package dbaudit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSink struct {
	mu      sync.Mutex
	rows    []store.DatabaseStatementAudit
	batches int
	pruned  time.Time
	trimmed int
	// failWhen makes an insert fail when it returns true for the batch.
	failWhen func([]store.DatabaseStatementAudit) bool
}

func (f *fakeSink) InsertDatabaseStatementAudits(_ context.Context, r []store.DatabaseStatementAudit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWhen != nil && f.failWhen(r) {
		return errors.New("insert refused")
	}
	f.rows = append(f.rows, r...)
	f.batches++
	return nil
}

func (f *fakeSink) TrimDatabaseStatementAudits(_ context.Context, maxRows, _ int) (int64, error) {
	f.trimmed = maxRows
	return 0, nil
}

func (f *fakeSink) PruneDatabaseStatementAudits(_ context.Context, c time.Time, _ int) (int64, error) {
	f.pruned = c
	return 3, nil
}

func (f *fakeSink) count() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows), f.batches
}

type fakeSet struct {
	text  bool
	queue int
	days  int
}

func (fakeSet) MaxRows(context.Context) int      { return 500 }
func (fakeSet) TextMaxBytes(context.Context) int { return 64 }

func (s fakeSet) StoreText(context.Context) bool    { return s.text }
func (s fakeSet) QueueSize(context.Context) int     { return s.queue }
func (s fakeSet) RetentionDays(context.Context) int { return s.days }

func TestAuditBatchesRowsIntoOneWrite(t *testing.T) {
	sink := &fakeSink{}
	w := New(sink, fakeSet{queue: 100}, Options{FlushInterval: time.Hour, FlushRows: 100})
	for i := 0; i < 10; i++ {
		w.Record(context.Background(), Event{DatabaseName: "app", Action: "db:read", Outcome: OutcomeOK})
	}
	w.Flush(context.Background())
	if n, b := sink.count(); n != 10 || b != 1 {
		t.Fatalf("rows %d batches %d", n, b)
	}
}

func TestAuditQueueFullDropsAndCounts(t *testing.T) {
	sink := &fakeSink{}
	w := New(sink, fakeSet{queue: 3}, Options{FlushInterval: time.Hour, FlushRows: 1000})
	for i := 0; i < 5; i++ {
		w.Record(context.Background(), Event{DatabaseName: "app"})
	}
	if w.Dropped() != 2 {
		t.Fatalf("dropped %d", w.Dropped())
	}
	var sb strings.Builder
	if err := w.WriteMetrics(&sb, "lr_"); err != nil || !strings.Contains(sb.String(), "lr_database_audit_dropped_total 2") {
		t.Fatalf("metrics %q %v", sb.String(), err)
	}
}

func TestAuditShutdownFlushes(t *testing.T) {
	sink := &fakeSink{}
	w := New(sink, fakeSet{queue: 100}, Options{FlushInterval: time.Hour, FlushRows: 1000})
	w.Start()
	w.Record(context.Background(), Event{DatabaseName: "app"})
	w.Close()
	if n, _ := sink.count(); n != 1 {
		t.Fatalf("rows after close %d", n)
	}
}

func TestAuditFlushOnRowThreshold(t *testing.T) {
	sink := &fakeSink{}
	w := New(sink, fakeSet{queue: 100}, Options{FlushInterval: time.Hour, FlushRows: 2})
	w.Start()
	defer w.Close()
	w.Record(context.Background(), Event{DatabaseName: "app"})
	w.Record(context.Background(), Event{DatabaseName: "app"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := sink.count(); n == 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("threshold flush did not happen")
}

func TestAuditStatementTextOnlyWhenSettingOn(t *testing.T) {
	for _, on := range []bool{false, true} {
		sink := &fakeSink{}
		w := New(sink, fakeSet{queue: 10, text: on}, Options{FlushInterval: time.Hour})
		w.Record(context.Background(), Event{DatabaseName: "app", StatementText: "select 'hostile-value'"})
		w.Flush(context.Background())
		got := sink.rows[0].StatementText
		if on != (got != "") {
			t.Fatalf("text setting %v stored %q", on, got)
		}
	}
}

func TestAuditSweepUsesRetention(t *testing.T) {
	sink := &fakeSink{}
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	n, err := Sweep(context.Background(), sink, fakeSet{days: 90}, now)
	if err != nil || n != 3 || !sink.pruned.Equal(now.AddDate(0, 0, -90)) {
		t.Fatalf("%d %v %v", n, err, sink.pruned)
	}
}

func TestAuditRowCapsFieldsAndMasksText(t *testing.T) {
	sink := &fakeSink{}
	w := New(sink, fakeSet{queue: 10, text: true}, Options{FlushInterval: time.Hour})
	long := strings.Repeat("x", 5000)
	w.Record(context.Background(), Event{DatabaseName: long, ClientKind: long, StatementText: "select '" + strings.Repeat("y", 500) + "'"})
	secretmask.Add("canary-registered-secret-9931")
	w.Record(context.Background(), Event{DatabaseName: "app", StatementText: "select 'canary-registered-secret-9931'"})
	w.Flush(context.Background())
	if got := sink.rows[0]; len(got.DatabaseName) != fieldMaxBytes || len(got.ClientKind) != fieldMaxBytes || len(got.StatementText) > 64 {
		t.Fatalf("not capped: %d %d %d", len(got.DatabaseName), len(got.ClientKind), len(got.StatementText))
	}
	if strings.Contains(sink.rows[1].StatementText, "canary-registered-secret-9931") {
		t.Fatalf("secret stored: %q", sink.rows[1].StatementText)
	}
}

func TestAuditFlushRetriesThenIsolatesBadRow(t *testing.T) {
	sink := &fakeSink{failWhen: func(b []store.DatabaseStatementAudit) bool {
		for _, r := range b {
			if r.DatabaseName == "bad" {
				return true
			}
		}
		return false
	}}
	w := New(sink, fakeSet{queue: 10}, Options{FlushInterval: time.Hour})
	for _, n := range []string{"a", "bad", "c"} {
		w.Record(context.Background(), Event{DatabaseName: n})
	}
	w.Flush(context.Background())
	if n, _ := sink.count(); n != 2 || w.Failed() != 1 {
		t.Fatalf("stored %d failed %d, want 2 and 1", n, w.Failed())
	}
}

func TestAuditCloseThenStartIsSafe(_ *testing.T) {
	w := New(&fakeSink{}, fakeSet{queue: 10}, Options{FlushInterval: time.Hour})
	w.Start()
	w.Close()
	w.Close()
	w.Start()
	w.Close()
}

func TestAuditSweepTrimsToRowCap(t *testing.T) {
	sink := &fakeSink{}
	if _, err := Sweep(context.Background(), sink, fakeSet{days: 90}, time.Now()); err != nil || sink.trimmed != 500 {
		t.Fatalf("trimmed %d %v", sink.trimmed, err)
	}
}
