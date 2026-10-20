package alerting

import (
	"context"
	"testing"
	"time"
)

func TestRecordAndListNotificationDeliveries_NewestFirst(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Team Slack", Kind: NotifySlack, NotifyURL: "https://hooks.slack.com/x", Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	if err := db.RecordNotificationDelivery(ctx, NotificationDelivery{ID: "ndl_1", ChannelID: "chn_1", Trigger: "test", Success: true}); err != nil {
		t.Fatalf("RecordNotificationDelivery(1) error = %v", err)
	}
	time.Sleep(2 * time.Millisecond) // created_at has sub-second resolution; force the two rows to sort deterministically
	if err := db.RecordNotificationDelivery(ctx, NotificationDelivery{ID: "ndl_2", ChannelID: "chn_1", Trigger: "deploy-failed", Success: false, Error: "receiver returned status 500"}); err != nil {
		t.Fatalf("RecordNotificationDelivery(2) error = %v", err)
	}

	got, err := db.ListNotificationDeliveries(ctx, "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "ndl_2" || got[1].ID != "ndl_1" {
		t.Fatalf("ListNotificationDeliveries() = %+v, want [ndl_2, ndl_1] newest first", got)
	}
	if got[0].Success || got[0].Error != "receiver returned status 500" {
		t.Errorf("got[0] = %+v, want the failed delivery with its error detail", got[0])
	}
	if !got[1].Success || got[1].Trigger != "test" {
		t.Errorf("got[1] = %+v, want the successful test delivery", got[1])
	}
}

func TestListNotificationDeliveries_ScopesByChannel(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "A", Kind: NotifyGeneric, NotifyURL: "https://a.example.com", Enabled: true}); err != nil {
		t.Fatalf("seed channel a: %v", err)
	}
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_2", Name: "B", Kind: NotifyGeneric, NotifyURL: "https://b.example.com", Enabled: true}); err != nil {
		t.Fatalf("seed channel b: %v", err)
	}
	if err := db.RecordNotificationDelivery(ctx, NotificationDelivery{ID: "ndl_1", ChannelID: "chn_1", Trigger: "test", Success: true}); err != nil {
		t.Fatalf("seed delivery a: %v", err)
	}
	if err := db.RecordNotificationDelivery(ctx, NotificationDelivery{ID: "ndl_2", ChannelID: "chn_2", Trigger: "test", Success: true}); err != nil {
		t.Fatalf("seed delivery b: %v", err)
	}

	got, err := db.ListNotificationDeliveries(ctx, "chn_1", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "ndl_1" {
		t.Errorf("ListNotificationDeliveries(chn_1) = %+v, want only ndl_1", got)
	}
}

func TestListNotificationDeliveries_RespectsLimit(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()

	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "A", Kind: NotifyGeneric, NotifyURL: "https://a.example.com", Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	for i := 0; i < 3; i++ {
		id, err := NewNotificationDeliveryID()
		if err != nil {
			t.Fatalf("NewNotificationDeliveryID() error = %v", err)
		}
		if err := db.RecordNotificationDelivery(ctx, NotificationDelivery{ID: id, ChannelID: "chn_1", Trigger: "test", Success: true}); err != nil {
			t.Fatalf("seed delivery %d: %v", i, err)
		}
	}

	got, err := db.ListNotificationDeliveries(ctx, "chn_1", 2, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListNotificationDeliveries(limit=2) returned %d rows, want 2", len(got))
	}
}

func TestListNotificationDeliveries_EmptyForUnknownChannel(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	got, err := db.ListNotificationDeliveries(context.Background(), "ghost", 50, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListNotificationDeliveries(ghost) = %+v, want empty", got)
	}
}

func TestRecordDelivery_NoopWhenChannelIDEmpty(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	recordDelivery(ctx, db, nil, "", "deploy-succeeded", nil)

	// No channel exists, so a non-noop call would have failed the FK
	// constraint; confirm no row was written for any channel at all by
	// checking the count directly.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries`).Scan(&count); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if count != 0 {
		t.Errorf("notification_deliveries has %d rows, want 0 for an empty channel id", count)
	}
}
