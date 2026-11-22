package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunbook_SavedShownInInboxAndNotification(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	r := Rule{ID: "r1", Name: "cpu", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent",
		Comparator: GreaterThan, Threshold: 80, Enabled: true, Runbook: "https://wiki.example.test/cpu"}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetRule(ctx, "r1")
	if err != nil || got.Runbook != r.Runbook {
		t.Fatalf("GetRule runbook = %q, err %v", got.Runbook, err)
	}

	seedHistory(t, db, time.Now().Add(-time.Minute), "web", SeverityWarning, EventFired, OutcomeSent)
	items, err := db.ListInbox(ctx, "user:a", InboxFilter{})
	if err != nil || len(items) != 1 || items[0].Runbook != r.Runbook {
		t.Fatalf("inbox runbook = %+v, err %v", items, err)
	}

	if text := summaryText(Event{Rule: *got}); !strings.Contains(text, "Runbook: "+r.Runbook) {
		t.Fatalf("firing text lacks runbook: %q", text)
	}
	if text := summaryText(Event{Rule: *got, Resolved: true}); strings.Contains(text, "Runbook") {
		t.Fatalf("resolved text carries runbook: %q", text)
	}
}
