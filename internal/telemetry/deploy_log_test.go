package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestWriteDeployLogBatch_EmptyIsNoop(t *testing.T) {
	db := newTestDB(t)
	if err := db.WriteDeployLogBatch(context.Background(), nil); err != nil {
		t.Errorf("WriteDeployLogBatch(nil) error = %v, want nil", err)
	}
}

func TestWriteAndQueryDeployLog_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	entries := []DeployLogEntry{
		{AttemptID: "dep_1", Stream: "stdout", Timestamp: base, Message: "[1/4] FROM golang:1.22"},
		{AttemptID: "dep_1", Stream: "stderr", Timestamp: base.Add(time.Second), Message: "warning: something"},
		{AttemptID: "dep_1", Stream: "stdout", Timestamp: base.Add(2 * time.Second), Message: "build complete"},
		// A different attempt's lines must never show up in dep_1's query.
		{AttemptID: "dep_2", Stream: "stdout", Timestamp: base, Message: "unrelated attempt"},
	}
	if err := db.WriteDeployLogBatch(ctx, entries); err != nil {
		t.Fatalf("WriteDeployLogBatch() error = %v", err)
	}

	got, err := db.QueryDeployLog(ctx, "dep_1")
	if err != nil {
		t.Fatalf("QueryDeployLog() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 lines for dep_1, got %d", len(got))
	}
	if got[0].Message != "[1/4] FROM golang:1.22" || got[1].Message != "warning: something" || got[2].Message != "build complete" {
		t.Errorf("lines not in chronological order: %+v", got)
	}
	if got[1].Stream != "stderr" {
		t.Errorf("Stream = %q, want %q", got[1].Stream, "stderr")
	}
}

func TestQueryDeployLog_NoRowsIsNotError(t *testing.T) {
	db := newTestDB(t)
	got, err := db.QueryDeployLog(context.Background(), "dep_never-written")
	if err != nil {
		t.Fatalf("QueryDeployLog() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no lines, got %d", len(got))
	}
}

func TestRetainDeployLogs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now()
	if err := db.WriteDeployLogBatch(ctx, []DeployLogEntry{
		{AttemptID: "dep_old", Stream: "stdout", Timestamp: old, Message: "old line"},
		{AttemptID: "dep_recent", Stream: "stdout", Timestamp: recent, Message: "recent line"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	deleted, err := db.RetainDeployLogs(ctx, cutoff)
	if err != nil {
		t.Fatalf("RetainDeployLogs() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	gotOld, err := db.QueryDeployLog(ctx, "dep_old")
	if err != nil {
		t.Fatalf("QueryDeployLog(dep_old) error = %v", err)
	}
	if len(gotOld) != 0 {
		t.Errorf("expected dep_old's line to be retained away, got %d", len(gotOld))
	}

	gotRecent, err := db.QueryDeployLog(ctx, "dep_recent")
	if err != nil {
		t.Fatalf("QueryDeployLog(dep_recent) error = %v", err)
	}
	if len(gotRecent) != 1 {
		t.Errorf("expected dep_recent's line to survive, got %d", len(gotRecent))
	}
}
