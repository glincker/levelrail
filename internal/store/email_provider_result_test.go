package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordEmailProviderResult(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertEmailProvider(ctx, EmailProvider{ID: "p1", Role: EmailRolePrimary, Backend: EmailBackendSMTP, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := db.RecordEmailProviderResult(ctx, "p1", false, "auth_failed", at); err != nil {
		t.Fatal(err)
	}
	p, _ := db.GetEmailProvider(ctx, "p1")
	if p.LastErrorKind != "auth_failed" || !p.LastOKAt.IsZero() {
		t.Fatalf("after failure: %+v", p)
	}
	if err := db.RecordEmailProviderResult(ctx, "p1", true, "", at); err != nil {
		t.Fatal(err)
	}
	p, _ = db.GetEmailProvider(ctx, "p1")
	if p.LastErrorKind != "" || !p.LastOKAt.Equal(at) {
		t.Fatalf("after success: %+v", p)
	}
	if err := db.RecordEmailProviderResult(ctx, "nope", true, "", at); !errors.Is(err, ErrEmailProviderNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestEmailDelivery_ProviderColumnsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	d := EmailDelivery{Provider: "smtp", Status: EmailDeliverySent, ProviderID: "b", FailoverFrom: "a"}
	if err := db.InsertEmailDelivery(ctx, d, "x@example.com", false); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListEmailDeliveries(ctx, EmailDeliveryFilter{})
	if err != nil || len(rows) != 1 || rows[0].ProviderID != "b" || rows[0].FailoverFrom != "a" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}
