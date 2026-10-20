package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSignatureDetector_CountsOnlyErrorsAbsentBeforeDeploy(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	deploy := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	line := func(at time.Time, msg string) LogEntry {
		return LogEntry{ResourceID: "service:web", Stream: "stderr", Timestamp: at, Message: msg}
	}
	entries := []LogEntry{
		line(deploy.Add(-5*time.Minute), "ERROR db timeout after 30s"),
		line(deploy.Add(2*time.Minute), "ERROR db timeout after 31s"),
		line(deploy.Add(3*time.Minute), "ERROR cannot render template home"),
		line(deploy.Add(4*time.Minute), "ERROR nil pointer in cart handler"),
		line(deploy.Add(5*time.Minute), "INFO warmed cache"),
	}
	if err := db.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatal(err)
	}

	d := NewSignatureDetector{Src: db, MaxRows: 100}
	got, err := d.NewErrorTypes(context.Background(), "web", deploy, deploy.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("new error types = %d, want 2 (the db timeout existed before the deploy)", got)
	}
}

func TestQueryLogsLimited_NewestRowsAndTruncation(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	var entries []LogEntry
	for i := 0; i < 10; i++ {
		entries = append(entries, LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(time.Duration(i) * time.Second), Message: "line " + string(rune('a'+i))})
	}
	if err := db.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	got, truncated, err := db.QueryLogsLimited(context.Background(), "service:web", base, base.Add(time.Minute), "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(got) != 4 || got[0].Message != "line g" || got[3].Message != "line j" {
		t.Fatalf("got %d rows truncated=%v first=%q last=%q", len(got), truncated, got[0].Message, got[len(got)-1].Message)
	}
	all, truncated, _ := db.QueryLogsLimited(context.Background(), "service:web", base, base.Add(time.Minute), "", 50)
	if truncated || len(all) != 10 {
		t.Fatalf("unbounded-enough read: %d rows truncated=%v", len(all), truncated)
	}
}
