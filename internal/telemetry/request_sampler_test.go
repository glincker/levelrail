package telemetry

import (
	"context"
	"testing"
	"time"
)

type fakeDrainer struct{ windows map[string]RequestWindow }

func (f *fakeDrainer) DrainByApp() map[string]RequestWindow {
	out := f.windows
	f.windows = nil
	return out
}

func TestRequestSampler_SampleOnce(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	src := &fakeDrainer{windows: map[string]RequestWindow{"web": {Requests: 4, Status2xx: 4}}}
	s := NewRequestSampler(src, db, time.Second, nil)

	if err := s.SampleOnce(ctx, at); err != nil {
		t.Fatal(err)
	}
	got, err := db.Query(ctx, "service:web", MetricHTTPRequests, at.Add(-time.Minute), at.Add(time.Minute))
	if err != nil || len(got) != 1 || got[0].Value != 4 {
		t.Errorf("stored = %v, %v", got, err)
	}
	if err := s.SampleOnce(ctx, at.Add(15*time.Second)); err != nil {
		t.Fatalf("idle tick must not error: %v", err)
	}
	after, _ := db.Query(ctx, "service:web", MetricHTTPRequests, at.Add(-time.Minute), at.Add(time.Minute))
	if len(after) != 1 {
		t.Errorf("idle tick wrote rows: %v", after)
	}
}
