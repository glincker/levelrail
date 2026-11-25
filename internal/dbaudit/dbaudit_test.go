package dbaudit

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSink struct {
	mu      sync.Mutex
	rows    []store.DatabaseStatementAudit
	batches int
	pruned  time.Time
}

func (f *fakeSink) InsertDatabaseStatementAudits(_ context.Context, r []store.DatabaseStatementAudit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, r...)
	f.batches++
	return nil
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
