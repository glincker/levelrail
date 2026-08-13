package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestClassifyLine(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		wantStructured bool
		wantFields     string
	}{
		{
			name:           "plain text line",
			raw:            "starting server on :8080",
			wantStructured: false,
			wantFields:     "",
		},
		{
			name:           "valid JSON object",
			raw:            `{"level":"info","msg":"ready"}`,
			wantStructured: true,
			wantFields:     `{"level":"info","msg":"ready"}`,
		},
		{
			name:           "JSON object with leading/trailing whitespace",
			raw:            `  {"level":"error","msg":"boom"}  `,
			wantStructured: true,
			wantFields:     `{"level":"error","msg":"boom"}`,
		},
		{
			name:           "bare JSON number is not structured, it's not what app-emits-JSON logging means",
			raw:            "42",
			wantStructured: false,
			wantFields:     "",
		},
		{
			name:           "bare JSON string is not structured",
			raw:            `"just a string"`,
			wantStructured: false,
			wantFields:     "",
		},
		{
			name:           "JSON array is not structured",
			raw:            `["a","b"]`,
			wantStructured: false,
			wantFields:     "",
		},
		{
			name:           "malformed JSON object",
			raw:            `{"level":"info"`,
			wantStructured: false,
			wantFields:     "",
		},
		{
			name:           "empty line",
			raw:            "",
			wantStructured: false,
			wantFields:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStructured, gotFields := classifyLine(tt.raw)
			if gotStructured != tt.wantStructured {
				t.Errorf("classifyLine() structured = %v, want %v", gotStructured, tt.wantStructured)
			}
			if gotFields != tt.wantFields {
				t.Errorf("classifyLine() fields = %q, want %q", gotFields, tt.wantFields)
			}
		})
	}
}

func TestFTSPhrase(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "plain word", query: "error", want: `"error"`},
		{name: "phrase with spaces", query: "connection refused", want: `"connection refused"`},
		{name: "embedded double quote is doubled", query: `say "hi"`, want: `"say ""hi"""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ftsPhrase(tt.query); got != tt.want {
				t.Errorf("ftsPhrase(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestWriteLogBatch_EmptyIsNoop(t *testing.T) {
	db := newTestDB(t)
	if err := db.WriteLogBatch(context.Background(), nil); err != nil {
		t.Errorf("WriteLogBatch(nil) error = %v, want nil", err)
	}
}

func TestWriteLogBatchAndQuery_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	base := time.Unix(1_700_000_000, 0).UTC()
	entries := []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base, Message: "server started"},
		{ResourceID: "service:web", Stream: "stderr", Timestamp: base.Add(time.Second), Message: "warning: slow query"},
		{ResourceID: "service:worker", Stream: "stdout", Timestamp: base, Message: "unrelated resource, must not leak into service:web's results"},
	}
	if err := db.WriteLogBatch(ctx, entries); err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	got, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryLogs() returned %d entries, want 2", len(got))
	}
	if got[0].Message != "server started" || got[1].Message != "warning: slow query" {
		t.Errorf("QueryLogs() messages = [%q, %q], want in timestamp order", got[0].Message, got[1].Message)
	}
	if got[0].Stream != "stdout" || got[1].Stream != "stderr" {
		t.Errorf("QueryLogs() streams = [%q, %q], want [stdout, stderr]", got[0].Stream, got[1].Stream)
	}
	if !got[0].Timestamp.Equal(base) {
		t.Errorf("QueryLogs()[0].Timestamp = %v, want %v", got[0].Timestamp, base)
	}
}

func TestQueryLogs_NoEntries_ReturnsEmptyNotError(t *testing.T) {
	db := newTestDB(t)
	got, err := db.QueryLogs(context.Background(), "service:nonexistent", time.Unix(0, 0), time.Now(), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v, want nil (no entries is a valid observed state)", err)
	}
	if len(got) != 0 {
		t.Errorf("QueryLogs() = %d entries, want 0", len(got))
	}
}

func TestQueryLogs_RespectsTimeRange(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	err := db.WriteLogBatch(ctx, []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(-time.Hour), Message: "too early"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base, Message: "in range"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(time.Hour), Message: "too late"},
	})
	if err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	got, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 1 || got[0].Message != "in range" {
		t.Errorf("QueryLogs() with a tight range = %+v, want exactly the one entry inside [from, to]", got)
	}
}

func TestQueryLogs_FullTextSearch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	err := db.WriteLogBatch(ctx, []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base, Message: "connection refused to database"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(time.Second), Message: "request completed in 12ms"},
		{ResourceID: "service:web", Stream: "stderr", Timestamp: base.Add(2 * time.Second), Message: "panic: nil pointer dereference"},
	})
	if err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	got, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "connection")
	if err != nil {
		t.Fatalf("QueryLogs() with query error = %v", err)
	}
	if len(got) != 1 || got[0].Message != "connection refused to database" {
		t.Errorf("QueryLogs(query=\"connection\") = %+v, want exactly the one matching entry", got)
	}

	gotPanic, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "panic")
	if err != nil {
		t.Fatalf("QueryLogs() with query error = %v", err)
	}
	if len(gotPanic) != 1 || gotPanic[0].Message != "panic: nil pointer dereference" {
		t.Errorf("QueryLogs(query=\"panic\") = %+v, want exactly the one matching entry", gotPanic)
	}

	gotNone, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "nonexistentword")
	if err != nil {
		t.Fatalf("QueryLogs() with query error = %v", err)
	}
	if len(gotNone) != 0 {
		t.Errorf("QueryLogs(query=\"nonexistentword\") = %+v, want 0 matches", gotNone)
	}
}

func TestQueryLogs_StructuredLinesRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	err := db.WriteLogBatch(ctx, []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base, Message: `{"level":"info","msg":"ready"}`, Structured: true, FieldsJSON: `{"level":"info","msg":"ready"}`},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(time.Second), Message: "plain text line"},
	})
	if err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	got, err := db.QueryLogs(ctx, "service:web", base.Add(-time.Minute), base.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryLogs() = %d entries, want 2", len(got))
	}
	if !got[0].Structured || got[0].FieldsJSON != `{"level":"info","msg":"ready"}` {
		t.Errorf("got[0] = %+v, want Structured=true with FieldsJSON preserved", got[0])
	}
	if got[1].Structured || got[1].FieldsJSON != "" {
		t.Errorf("got[1] = %+v, want Structured=false with empty FieldsJSON", got[1])
	}
}

func TestRetainLogs_DeletesOnlyOlderThanCutoff(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	err := db.WriteLogBatch(ctx, []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(-48 * time.Hour), Message: "old, should go"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(-1 * time.Hour), Message: "recent, should stay"},
	})
	if err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	deleted, err := db.RetainLogs(ctx, base.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("RetainLogs() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("RetainLogs() deleted = %d, want 1", deleted)
	}

	remaining, err := db.QueryLogs(ctx, "service:web", base.Add(-72*time.Hour), base, "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Message != "recent, should stay" {
		t.Errorf("remaining entries after RetainLogs() = %+v, want exactly the recent one", remaining)
	}
}

func TestRetainLogs_KeepsFTSIndexInSync(t *testing.T) {
	// A deleted row must not leave a stale, still-matchable entry behind
	// in log_entries_fts: proves the log_entries_ad trigger
	// (migrations/0002_log_entries.sql) actually fires, not just that
	// RetainLogs deletes from log_entries itself.
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	if err := db.WriteLogBatch(ctx, []LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: base.Add(-48 * time.Hour), Message: "uniquetoken-for-retention-test"},
	}); err != nil {
		t.Fatalf("WriteLogBatch() error = %v", err)
	}

	if _, err := db.RetainLogs(ctx, base.Add(-24*time.Hour)); err != nil {
		t.Fatalf("RetainLogs() error = %v", err)
	}

	got, err := db.QueryLogs(ctx, "service:web", base.Add(-72*time.Hour), base, "uniquetoken-for-retention-test")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("QueryLogs() found %d matches for a deleted row's text, want 0 (stale FTS entry left behind)", len(got))
	}
}

// fakeLogSource is a scriptable LogSource: each container ID maps to a
// fixed slice of lines delivered once, then the channel closes, mirroring
// a container that produced some output and stopped.
type fakeLogSource struct {
	lines map[string][]docker.LogLine
}

func (f *fakeLogSource) Logs(ctx context.Context, containerID string, _ bool, _ time.Time) (<-chan docker.LogLine, <-chan error) {
	out := make(chan docker.LogLine)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		for _, l := range f.lines[containerID] {
			select {
			case out <- l:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, errCh
}

func TestStreamOne_WritesAllLinesAfterStreamCloses(t *testing.T) {
	db := newTestDB(t)
	source := &fakeLogSource{
		lines: map[string][]docker.LogLine{
			"c1": {
				{Stream: "stdout", Message: "line one"},
				{Stream: "stdout", Message: "line two"},
				{Stream: "stderr", Message: "line three"},
			},
		},
	}
	lc := NewLogCollector(source, db, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := lc.StreamOne(ctx, LogTarget{ResourceID: "service:web", ContainerID: "c1"})
	if err != nil {
		t.Fatalf("StreamOne() error = %v, want nil (a clean stream end)", err)
	}

	got, err := db.QueryLogs(context.Background(), "service:web", time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("QueryLogs() = %d entries, want 3 (the batch must flush when the stream closes, even under logBatchMaxLines)", len(got))
	}
}

func TestStreamOne_FlushesOnLineCountThreshold(t *testing.T) {
	db := newTestDB(t)
	lines := make([]docker.LogLine, logBatchMaxLines+5)
	for i := range lines {
		lines[i] = docker.LogLine{Stream: "stdout", Message: "line"}
	}
	source := &fakeLogSource{lines: map[string][]docker.LogLine{"c1": lines}}
	lc := NewLogCollector(source, db, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := lc.StreamOne(ctx, LogTarget{ResourceID: "service:web", ContainerID: "c1"}); err != nil {
		t.Fatalf("StreamOne() error = %v", err)
	}

	got, err := db.QueryLogs(context.Background(), "service:web", time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != len(lines) {
		t.Fatalf("QueryLogs() = %d entries, want %d (a mid-stream flush at logBatchMaxLines must not drop the tail batch)", len(got), len(lines))
	}
}

func TestStreamOne_ContextCancelled_FlushesPartialBatch(t *testing.T) {
	db := newTestDB(t)
	// A source whose channel never closes on its own: StreamOne must be
	// the one to notice ctx cancellation and flush what it has so far,
	// not wait forever for the stream to end.
	lines := make(chan docker.LogLine)
	errs := make(chan error)
	source := &blockingLogSource{lines: lines, errs: errs}
	lc := NewLogCollector(source, db, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- lc.StreamOne(ctx, LogTarget{ResourceID: "service:web", ContainerID: "c1"}) }()

	lines <- docker.LogLine{Stream: "stdout", Message: "buffered before cancel"}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("StreamOne() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StreamOne() did not return after ctx cancellation")
	}

	got, err := db.QueryLogs(context.Background(), "service:web", time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 1 || got[0].Message != "buffered before cancel" {
		t.Errorf("QueryLogs() = %+v, want the one line buffered before cancellation to have been flushed", got)
	}
}

type blockingLogSource struct {
	lines chan docker.LogLine
	errs  chan error
}

func (b *blockingLogSource) Logs(context.Context, string, bool, time.Time) (<-chan docker.LogLine, <-chan error) {
	return b.lines, b.errs
}

func TestLogCollectorRun_StartsAndStopsStreamsAsTargetsChange(t *testing.T) {
	db := newTestDB(t)
	source := &fakeLogSource{
		lines: map[string][]docker.LogLine{
			"c1": {{Stream: "stdout", Message: "from c1"}},
		},
	}
	lc := NewLogCollector(source, db, nil)

	var call int
	targetsFunc := func(context.Context) ([]LogTarget, error) {
		call++
		if call == 1 {
			return []LogTarget{{ResourceID: "service:web", ContainerID: "c1"}}, nil
		}
		return nil, nil // second resync: target gone, e.g. the container was removed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := lc.Run(ctx, 20*time.Millisecond, targetsFunc)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if call < 2 {
		t.Errorf("targetsFunc called %d times, want at least 2 (fresh lookup every resync)", call)
	}

	got, err := db.QueryLogs(context.Background(), "service:web", time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 1 || got[0].Message != "from c1" {
		t.Errorf("QueryLogs() = %+v, want the one line c1's short-lived stream produced", got)
	}
}

func TestLogCollectorRun_TargetsFuncError_ContinuesToNextResync(t *testing.T) {
	db := newTestDB(t)
	lc := NewLogCollector(&fakeLogSource{}, db, nil)

	var call int
	targetsFunc := func(context.Context) ([]LogTarget, error) {
		call++
		return nil, errors.New("store unreachable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()

	err := lc.Run(ctx, 10*time.Millisecond, targetsFunc)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded (a targetsFunc error must not stop the loop)", err)
	}
	if call < 2 {
		t.Errorf("targetsFunc called %d times, want at least 2: a failing resync must not stop future attempts", call)
	}
}
