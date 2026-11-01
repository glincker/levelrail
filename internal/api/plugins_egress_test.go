package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

func TestSummarizeDenialsTotalsByReasonAndLatest(t *testing.T) {
	got := summarizeDenials([]egress.DenialCount{
		{Plugin: "p", Reason: "host_not_allowed", Count: 3, LastDenied: "2026-10-31T10:00:00Z"},
		{Plugin: "p", Reason: "blocked_address", Count: 1, LastDenied: "2026-10-31T11:00:00Z"},
	})
	if got.Total != 4 || got.ByReason["host_not_allowed"] != 3 || got.LastDenied != "2026-10-31T11:00:00Z" {
		t.Fatalf("summary = %+v", got)
	}
	if empty := summarizeDenials(nil); empty.Total != 0 || empty.ByReason == nil {
		t.Fatalf("empty summary = %+v", empty)
	}
}
