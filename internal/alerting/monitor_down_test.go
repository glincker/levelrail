package alerting

import (
	"context"
	"testing"
	"time"
)

func TestMonitorDownFiresAndResolves(t *testing.T) {
	tr := NewMonitorDownTracker()
	now := time.Now()
	r := Rule{ID: "r", Kind: KindMonitorDown}
	next, msg, _ := EvaluateMonitorDown(context.Background(), tr, r, now)
	if next.Firing || msg != "" {
		t.Fatalf("idle: %+v %q", next.Firing, msg)
	}
	tr.Record("m1", "site", "timeout", true, now)
	next, msg, _ = EvaluateMonitorDown(context.Background(), tr, r, now)
	if !next.Firing || msg != "monitor down: site: timeout" {
		t.Fatalf("down: %v %q", next.Firing, msg)
	}
	tr.Record("m1", "site", "", false, now)
	next, _, _ = EvaluateMonitorDown(context.Background(), tr, next, now.Add(time.Minute))
	if next.Firing {
		t.Fatal("must resolve on confirmed recovery")
	}
}
