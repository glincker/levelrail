package alerting

import (
	"testing"
	"time"
)

func TestChannelDigestBatchesWarnings(t *testing.T) {
	now := time.Now()
	d := NewChannelDigest(10 * time.Minute)
	for i := 0; i < 5; i++ {
		if !d.Offer("chn_a", SeverityWarning, "warn", now.Add(time.Duration(i)*time.Minute)) {
			t.Fatal("warning not held")
		}
	}
	if d.Offer("chn_a", SeverityCritical, "page", now) {
		t.Fatal("critical must bypass the digest")
	}
	if got := d.Flush(now.Add(5 * time.Minute)); len(got) != 0 {
		t.Fatalf("flushed before window: %d", len(got))
	}
	got := d.Flush(now.Add(11 * time.Minute))
	if len(got) != 1 || got[0].Count != 5 {
		t.Fatalf("want one delivery of 5, got %+v", got)
	}
	if again := d.Flush(now.Add(20 * time.Minute)); len(again) != 0 {
		t.Fatal("batch not cleared")
	}
}
