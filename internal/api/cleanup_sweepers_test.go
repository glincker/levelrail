package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestWebhookDeliverySweeperHonoursSavedRetention(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	old := store.WebhookDelivery{ID: "whd_old", ServiceName: "s", ReceivedAt: time.Now().Add(-60 * 24 * time.Hour)}
	if err := db.SaveWebhookDelivery(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyWebhookDeliveryDays, json.RawMessage("90"), "test"); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldWebhookDeliveries(ctx); err != nil || n != 0 {
		t.Fatalf("with a saved 90 days the 60 day row must stay: %d, %v", n, err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyWebhookDeliveryDays, json.RawMessage("30"), "test"); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldWebhookDeliveries(ctx); err != nil || n != 1 {
		t.Fatalf("with a saved 30 days the row must go: %d, %v", n, err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyWebhookDeliveryDays, json.RawMessage("0"), "test"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveWebhookDelivery(ctx, old); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldWebhookDeliveries(ctx); err != nil || n != 0 {
		t.Fatalf("0 keeps everything: %d, %v", n, err)
	}
}

func TestAuditSweeperHonoursSavedWindowsAndFloors(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	save := func(id string, ageDays int, action string) {
		e := store.AuditEntry{ID: id, ActorType: "system", Method: "GET", Path: "/x", StatusCode: 200, ClientKind: "system", Action: action,
			CreatedAt: store.FormatAuditTime(time.Now().Add(-time.Duration(ageDays)*24*time.Hour - time.Hour))}
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	save("aud_general_100", 100, "")
	save("aud_security_100", 100, "sensitive_read")
	save("aud_security_400", 400, "sensitive_read")

	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyAuditLogDays, json.RawMessage("180"), "test"); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldAuditEntries(ctx); err != nil || n != 1 {
		t.Fatalf("saved 180 days: removed %d, %v; only the 400 day security row is past its 365 day window", n, err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyAuditLogDays, json.RawMessage("90"), "test"); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldAuditEntries(ctx); err != nil || n != 1 {
		t.Fatalf("saved 90 days: removed %d, %v; the general 100 day row goes, the security one stays", n, err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyAuditLogDays, json.RawMessage("60"), "test"); err == nil {
		t.Fatal("a 60 day audit window is below the floor and must be refused")
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyAuditLogDays, json.RawMessage("0"), "test"); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.PurgeOldAuditEntries(ctx); err != nil || n != 0 {
		t.Fatalf("0 keeps the audit log: %d, %v", n, err)
	}
}
