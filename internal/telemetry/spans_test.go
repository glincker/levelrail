package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestSpansStoreListAndGet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	ms := int64(time.Millisecond)
	spans := []Span{
		{TraceID: "t1", SpanID: "a", ResourceID: "service:web", Name: "GET /", StartNs: base.UnixNano(), EndNs: base.UnixNano() + 200*ms},
		{TraceID: "t1", SpanID: "b", ParentID: "a", ResourceID: "service:web", Name: "db", StartNs: base.UnixNano() + 10*ms, EndNs: base.UnixNano() + 150*ms, StatusCode: 2},
		{TraceID: "t2", SpanID: "c", ResourceID: "service:web", Name: "GET /fast", StartNs: base.UnixNano() + 5*int64(time.Second), EndNs: base.UnixNano() + 5*int64(time.Second) + 5*ms},
		{TraceID: "t3", SpanID: "d", ResourceID: "service:other", Name: "GET /x", StartNs: base.UnixNano(), EndNs: base.UnixNano() + ms},
	}
	if err := db.WriteSpans(ctx, spans); err != nil {
		t.Fatal(err)
	}
	if err := db.WriteSpans(ctx, spans); err != nil { // a retried export is harmless
		t.Fatal(err)
	}

	all, err := db.ListTraces(ctx, TraceFilter{ResourceID: "service:web", From: base.Add(-time.Minute)})
	if err != nil || len(all) != 2 {
		t.Fatalf("traces = %+v err=%v, want 2 for service:web", all, err)
	}
	if all[0].TraceID != "t2" || all[1].TraceID != "t1" {
		t.Errorf("order = %s,%s want newest first", all[0].TraceID, all[1].TraceID)
	}
	if all[1].RootName != "GET /" || all[1].Spans != 2 || all[1].Errors != 1 || all[1].DurationNs != 200*ms {
		t.Errorf("t1 summary = %+v", all[1])
	}

	slow, _ := db.ListTraces(ctx, TraceFilter{ResourceID: "service:web", MinMillis: 100})
	if len(slow) != 1 || slow[0].TraceID != "t1" {
		t.Errorf("slow = %+v, want only t1", slow)
	}
	failed, _ := db.ListTraces(ctx, TraceFilter{ResourceID: "service:web", ErrorsOnly: true})
	if len(failed) != 1 || failed[0].TraceID != "t1" {
		t.Errorf("errors = %+v, want only t1", failed)
	}

	got, err := db.GetTrace(ctx, "service:web", "t1")
	if err != nil || len(got) != 2 || got[0].SpanID != "a" {
		t.Fatalf("trace = %+v err=%v", got, err)
	}
	if other, _ := db.GetTrace(ctx, "service:web", "t3"); len(other) != 0 {
		t.Errorf("another app's trace leaked: %+v", other)
	}

	n, err := db.RetainSpans(ctx, base.Add(time.Second))
	if err != nil || n != 3 {
		t.Errorf("retained deleted %d (err %v), want 3 old spans", n, err)
	}
}
