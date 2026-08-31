package store

import (
	"bytes"
	"context"
	"errors"
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
