package store

import (
	"context"
	"testing"
	"time"
)

func TestIngressSettings_DNSAutomationFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := IngressSettings{AppsBaseDomain: "apps.example.com", DNSCNAMETarget: "edge.example.net", DNSTTLSeconds: 120, DNSProxied: true}
	if err := db.UpdateIngressSettings(ctx, want); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := db.GetIngressSettings(ctx)
	if err != nil || got != want {
		t.Fatalf("got %+v err %v, want %+v", got, err, want)
	}
	if err := db.UpdateIngressSettings(ctx, IngressSettings{DNSTTLSeconds: MaxDNSTTLSeconds + 1}); err == nil {
		t.Fatal("an out of range ttl must be rejected")
	}
}

func TestManagedDNSRecords(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	rec := ManagedDNSRecord{Domain: "app.example.com", Provider: "cloudflare", Zone: "example.com.", Name: "app", RecordType: "A", Value: "203.0.113.5", AppName: "web"}
	if err := db.SaveManagedDNSRecord(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.Value = "203.0.113.6"
	if err := db.SaveManagedDNSRecord(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListManagedDNSRecords(ctx, "app.example.com")
	if err != nil || len(rows) != 1 || rows[0].Value != "203.0.113.6" {
		t.Fatalf("rows = %+v err = %v, want one upserted row", rows, err)
	}
	if err := db.DeleteManagedDNSRecord(ctx, "app.example.com", "A"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.ListManagedDNSRecords(ctx, "app.example.com"); len(rows) != 0 {
		t.Fatalf("rows after delete = %+v", rows)
	}
}

func TestDomainAutomationPolicyAndRuns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if raw, err := db.GetDomainAutomationRaw(ctx); err != nil || raw != "" {
		t.Fatalf("default policy = %q err = %v, want empty", raw, err)
	}
	if err := db.SetDomainAutomationRaw(ctx, `{"auto_dns":true}`); err != nil {
		t.Fatal(err)
	}
	if raw, _ := db.GetDomainAutomationRaw(ctx); raw != `{"auto_dns":true}` {
		t.Fatalf("policy = %q", raw)
	}

	id, err := NewDomainAutomationRunID()
	if err != nil {
		t.Fatal(err)
	}
	run := DomainAutomationRun{ID: id, AppName: "web", Domain: "app.example.com", Result: "live", Steps: "[]", Undo: "[]", CreatedAt: time.Now()}
	if err := db.SaveDomainAutomationRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkDomainAutomationRunUndone(ctx, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDomainAutomationRun(ctx, id)
	if err != nil || got.UndoneAt == nil || got.Domain != "app.example.com" {
		t.Fatalf("run = %+v err = %v", got, err)
	}
	if _, err := db.GetDomainAutomationRun(ctx, "missing"); err != ErrAutomationRunNotFound {
		t.Fatalf("missing run error = %v", err)
	}
	if runs, err := db.ListDomainAutomationRuns(ctx, 5); err != nil || len(runs) != 1 {
		t.Fatalf("runs = %+v err = %v", runs, err)
	}
}
