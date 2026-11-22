package ingress

import (
	"testing"
	"time"
)

func TestMeasureCutoverGap(t *testing.T) {
	base := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	recs := []AccessRecord{
		{Time: base, Status: 200, Millis: 20, RequestID: "ok"},
		{Time: base.Add(1 * time.Second), Status: 502, Millis: 5, RequestID: "a"},
		{Time: base.Add(1 * time.Second), Status: 502, Millis: 5, RequestID: "a"},
		{Time: base.Add(3 * time.Second), Status: 200, Millis: 2500, RequestID: "slow"},
		{Time: base.Add(4 * time.Second), Status: 500, Millis: 10, RequestID: "app500"},
	}
	g := MeasureCutoverGap(recs, 15*time.Second, time.Second)
	if g.Failed != 1 || g.Queued != 1 {
		t.Fatalf("failed=%d queued=%d, want 1 and 1", g.Failed, g.Queued)
	}
	// slow request started at base+0.5s, last end is base+3s
	if g.GapMS != 2500 {
		t.Fatalf("gap_ms=%d, want 2500", g.GapMS)
	}
	if clean := MeasureCutoverGap(recs[:1], time.Second, time.Second); clean.GapMS != 0 || clean.Failed != 0 {
		t.Fatalf("clean window must report zero gap: %+v", clean)
	}
}
