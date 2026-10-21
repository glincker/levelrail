package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEmailDeliveries_RetentionPrunesOnlyOldRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	for i, age := range []time.Duration{48 * time.Hour, 10 * 24 * time.Hour, time.Hour} {
		d := EmailDelivery{CreatedAt: now.Add(-age), Provider: "smtp", Status: EmailDeliverySent, MessageID: "m" + string(rune('a'+i))}
		if err := db.InsertEmailDelivery(ctx, d, "person@example.com", false); err != nil {
			t.Fatalf("InsertEmailDelivery() error = %v", err)
		}
	}
	n, err := db.PruneEmailDeliveries(ctx, now.Add(-7*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("PruneEmailDeliveries() = %d, %v, want 1 row removed", n, err)
	}
	rows, err := db.ListEmailDeliveries(ctx, EmailDeliveryFilter{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListEmailDeliveries() = %d rows, %v, want 2", len(rows), err)
	}
}

func TestEmailDeliveries_RecipientIsTruncatedUnlessAllowed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.InsertEmailDelivery(ctx, EmailDelivery{Provider: "ses", Status: EmailDeliveryFailed, ErrorKind: "auth_failed"}, "person@example.com", false); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(ctx, EmailDeliveryFilter{Query: "person@example.com", Status: EmailDeliveryFailed})
	if err != nil || len(rows) != 1 {
		t.Fatalf("search by full address = %d rows, %v, want 1 (matched through the hash)", len(rows), err)
	}
	if strings.Contains(rows[0].ToDisplay, "person") || rows[0].ToDisplay != "p***@example.com" {
		t.Errorf("to_display = %q, want a truncated address", rows[0].ToDisplay)
	}
}
