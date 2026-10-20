package telemetry

import (
	"testing"
	"time"
)

func TestRankLogsByRelevance(t *testing.T) {
	base := time.Unix(1700000000, 0)
	entries := []LogEntry{
		{Timestamp: base, Message: "error in a very long line with many unrelated words around it here"},
		{Timestamp: base.Add(time.Second), Message: "error error"},
		{Timestamp: base.Add(2 * time.Second), Message: "error timeout"},
	}
	tests := []struct {
		name  string
		query string
		first string
	}{
		{"term frequency wins", "error", "error error"},
		{"both terms win", "error timeout", "error timeout"},
		{"negated term does not score", "error NOT timeout", "error error"},
		{"no query falls back to newest", "", "error timeout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RankLogsByRelevance(entries, tc.query)
			if got[0].Message != tc.first {
				t.Errorf("first = %q, want %q", got[0].Message, tc.first)
			}
			if len(got) != len(entries) {
				t.Errorf("len = %d", len(got))
			}
		})
	}
}

func TestLogBroadcasterCountsDroppedLines(t *testing.T) {
	b := NewLogBroadcaster()
	_, takeDropped, unsubscribe := b.SubscribeCounted("service:web")
	defer unsubscribe()
	for i := 0; i < broadcastBufferSize+7; i++ {
		b.Publish(LogEntry{ResourceID: "service:web", Message: "flood"})
	}
	if got := takeDropped(); got != 7 {
		t.Fatalf("dropped = %d, want 7", got)
	}
	if got := takeDropped(); got != 0 {
		t.Fatalf("dropped after reset = %d, want 0", got)
	}
}
