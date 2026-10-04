package store

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewWebhookDeliveryID(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		id, err := NewWebhookDeliveryID()
		if err != nil {
			t.Fatalf("NewWebhookDeliveryID() error = %v", err)
		}
		if id[:len(webhookDeliveryIDPrefix)] != webhookDeliveryIDPrefix {
			t.Errorf("NewWebhookDeliveryID() = %q, want prefix %q", id, webhookDeliveryIDPrefix)
		}
		if seen[id] {
			t.Fatalf("NewWebhookDeliveryID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestSaveAndGetWebhookDelivery(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	received := time.Now().UTC().Truncate(time.Millisecond)
	want := WebhookDelivery{
		ID:             "whd_test1",
		ServiceName:    "web",
		Provider:       "github",
		EventType:      "push",
		HeaderFields:   map[string]string{"X-GitHub-Event": "push"},
		SignatureValid: true,
		Matched:        true,
		StatusCode:     200,
		Payload:        []byte(`{"ref":"refs/heads/main"}`),
		Error:          "",
		ReceivedAt:     received,
	}
	if err := db.SaveWebhookDelivery(ctx, want); err != nil {
		t.Fatalf("SaveWebhookDelivery() error = %v", err)
	}

	got, err := db.GetWebhookDelivery(ctx, "whd_test1")
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}
	if got.ID != want.ID || got.ServiceName != want.ServiceName || got.Provider != want.Provider ||
		got.EventType != want.EventType || got.SignatureValid != want.SignatureValid || got.Matched != want.Matched ||
		got.StatusCode != want.StatusCode {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !bytes.Equal(got.Payload, want.Payload) {
		t.Errorf("Payload = %q, want %q", got.Payload, want.Payload)
	}
	if got.PayloadTruncated {
		t.Error("PayloadTruncated = true, want false for a small payload")
	}
	if got.HeaderFields["X-GitHub-Event"] != "push" {
		t.Errorf("HeaderFields[X-GitHub-Event] = %q, want %q", got.HeaderFields["X-GitHub-Event"], "push")
	}
	if !got.ReceivedAt.Equal(want.ReceivedAt) {
		t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, want.ReceivedAt)
	}
}

func TestGetWebhookDelivery_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetWebhookDelivery(context.Background(), "whd_missing")
	if !errors.Is(err, ErrWebhookDeliveryNotFound) {
		t.Fatalf("GetWebhookDelivery() error = %v, want ErrWebhookDeliveryNotFound", err)
	}
}

func TestSaveWebhookDelivery_TruncatesOversizedPayload(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	oversized := bytes.Repeat([]byte("a"), MaxWebhookDeliveryPayloadBytes+100)
	if err := db.SaveWebhookDelivery(ctx, WebhookDelivery{
		ID:          "whd_big",
		ServiceName: "web",
		Provider:    "github",
		EventType:   "push",
		Payload:     oversized,
		ReceivedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveWebhookDelivery() error = %v", err)
	}

	got, err := db.GetWebhookDelivery(ctx, "whd_big")
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}
	if len(got.Payload) != MaxWebhookDeliveryPayloadBytes {
		t.Errorf("len(Payload) = %d, want %d", len(got.Payload), MaxWebhookDeliveryPayloadBytes)
	}
	if !got.PayloadTruncated {
		t.Error("PayloadTruncated = false, want true for an oversized payload")
	}
}

func TestDeleteWebhookDeliveriesOlderThan(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	seed := func(id string, receivedAt time.Time) {
		if err := db.SaveWebhookDelivery(ctx, WebhookDelivery{
			ID: id, ServiceName: "web", Provider: "github", EventType: "push",
			ReceivedAt: receivedAt,
		}); err != nil {
			t.Fatalf("SaveWebhookDelivery(%q) error = %v", id, err)
		}
	}
	seed("whd_old", old)
	seed("whd_recent", recent)

	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	n, err := db.DeleteWebhookDeliveriesOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteWebhookDeliveriesOlderThan() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}

	if _, err := db.GetWebhookDelivery(ctx, "whd_old"); !errors.Is(err, ErrWebhookDeliveryNotFound) {
		t.Errorf("GetWebhookDelivery(whd_old) error = %v, want ErrWebhookDeliveryNotFound", err)
	}
	if _, err := db.GetWebhookDelivery(ctx, "whd_recent"); err != nil {
		t.Errorf("GetWebhookDelivery(whd_recent) error = %v, want nil (should survive the purge)", err)
	}
}

func TestListWebhookDeliveries_NewestFirstAndScoped(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	seed := func(id, service string, offset time.Duration) {
		if err := db.SaveWebhookDelivery(ctx, WebhookDelivery{
			ID: id, ServiceName: service, Provider: "github", EventType: "push",
			ReceivedAt: base.Add(offset),
		}); err != nil {
			t.Fatalf("SaveWebhookDelivery(%q) error = %v", id, err)
		}
	}
	seed("whd_web_1", "web", 0)
	seed("whd_web_2", "web", time.Minute)
	seed("whd_web_3", "web", 2*time.Minute)
	seed("whd_other_1", "other", time.Minute)

	got, err := db.ListWebhookDeliveries(ctx, "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].ID != "whd_web_3" || got[1].ID != "whd_web_2" || got[2].ID != "whd_web_1" {
		t.Errorf("order = [%s, %s, %s], want newest first", got[0].ID, got[1].ID, got[2].ID)
	}

	limited, err := db.ListWebhookDeliveries(ctx, "web", 1, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries(limit=1) error = %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "whd_web_3" {
		t.Errorf("ListWebhookDeliveries(limit=1) = %+v, want just whd_web_3", limited)
	}

	before := base.Add(2 * time.Minute)
	paged, err := db.ListWebhookDeliveries(ctx, "web", 10, &before)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries(before) error = %v", err)
	}
	if len(paged) != 2 || paged[0].ID != "whd_web_2" || paged[1].ID != "whd_web_1" {
		t.Errorf("ListWebhookDeliveries(before) = %+v, want [whd_web_2, whd_web_1]", paged)
	}
}

// TestListWebhookDeliveries_UsesCoveringIndex proves migrations/0282's
// composite index lets ListWebhookDeliveries' WHERE+ORDER BY query plan
// skip a sort step (migrations/0068 only indexed service_name alone,
// which can't cover the received_at DESC ordering and forces a temp
// b-tree sort as the table grows).
func TestListWebhookDeliveries_UsesCoveringIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `
		EXPLAIN QUERY PLAN
		SELECT id, service_name, received_at FROM webhook_deliveries
		WHERE service_name = ? ORDER BY received_at DESC LIMIT ?
	`, "web", 20)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var plan string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan query plan row: %v", err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate query plan rows: %v", err)
	}

	if strings.Contains(plan, "TEMP B-TREE") {
		t.Errorf("query plan uses a temp b-tree sort, want the composite index to cover the ORDER BY:\n%s", plan)
	}
	if !strings.Contains(plan, "idx_webhook_deliveries_service_received") {
		t.Errorf("query plan doesn't use idx_webhook_deliveries_service_received:\n%s", plan)
	}
}
