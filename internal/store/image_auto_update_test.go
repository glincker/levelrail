package store

import (
	"testing"
	"time"
)

func TestImageAutoUpdate(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	if u, err := db.GetImageAutoUpdate(ctx, "web"); err != nil || u.Enabled {
		t.Fatalf("missing row = %+v, %v; want disabled", u, err)
	}
	if err := db.SetImageAutoUpdate(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetImageAutoUpdate(ctx, "api", false); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if err := db.RecordImageAutoUpdateCheck(ctx, "web", "up to date", at); err != nil {
		t.Fatal(err)
	}
	u, err := db.GetImageAutoUpdate(ctx, "web")
	if err != nil || !u.Enabled || u.LastResult != "up to date" || u.LastCheckedAt == nil || !u.LastCheckedAt.Equal(at) {
		t.Fatalf("web = %+v, %v", u, err)
	}
	if err := db.SetImageAutoUpdateWebhookHash(ctx, "web", "h1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := db.GetImageAutoUpdate(ctx, "web"); u.WebhookHash != "h1" || !u.Enabled {
		t.Fatalf("webhook set must keep enabled: %+v", u)
	}
	list, err := db.ListEnabledImageAutoUpdates(ctx)
	if err != nil || len(list) != 1 || list[0].ServiceName != "web" {
		t.Fatalf("enabled = %+v, %v; want only web", list, err)
	}
}
