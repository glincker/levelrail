package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestWebhookDeliveryExcerptsMaskRegisteredSecret(t *testing.T) {
	canary := "delivery-excerpt-secret-55"
	secretmask.Add(canary)
	db := openTestDB(t)
	ctx := context.Background()
	d := WebhookDelivery{ID: "whd_mask", ServiceName: "web", Provider: "github", EventType: "push",
		HeaderFields: map[string]string{}, Payload: []byte(`{}`), ReceivedAt: time.Now().UTC(),
		Error: "upstream said token=" + canary + " rejected", Note: "note " + canary + " tail"}
	if err := db.SaveWebhookDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetWebhookDelivery(ctx, "whd_mask")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Error+got.Note, canary) || !strings.Contains(got.Error, "rejected") || !strings.Contains(got.Note, "tail") {
		t.Fatalf("error=%q note=%q", got.Error, got.Note)
	}
}
