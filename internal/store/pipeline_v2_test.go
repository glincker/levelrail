package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedPipelineRun(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: "x", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePipelineRun(ctx, PipelineRun{ID: "r1", PipelineID: "p1", AppName: "web", TriggerKind: "webhook", Definition: "x", CreatedAt: now, TriggerPayload: `{"a":1}`, TriggerContentType: "application/json"}); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineRunKeepsWebhookPayload(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	seedPipelineRun(t, db)
	got, err := db.GetPipelineRun(context.Background(), "r1")
	if err != nil || got.TriggerPayload != `{"a":1}` || got.TriggerContentType != "application/json" {
		t.Fatalf("run = %+v, %v", got, err)
	}
}

func TestPipelineArtifactsUpsertAndExpiry(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	seedPipelineRun(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	a := PipelineArtifact{ID: "a1", RunID: "r1", JobKey: "build", Name: "build", SizeBytes: 3, SHA256: "aa", StoragePath: "r1/a1.tar.gz", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)}
	if err := db.SavePipelineArtifact(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.ID, a.SHA256, a.ExpiresAt = "a2", "bb", now.Add(time.Hour)
	if err := db.SavePipelineArtifact(ctx, a); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListPipelineArtifacts(ctx, "r1")
	if len(list) != 1 || list[0].ID != "a2" || list[0].SHA256 != "bb" {
		t.Fatalf("a retried job must replace its artifact: %+v", list)
	}
	if exp, _ := db.ListExpiredPipelineArtifacts(ctx, now); len(exp) != 0 {
		t.Errorf("expired = %+v", exp)
	}
	if exp, _ := db.ListExpiredPipelineArtifacts(ctx, now.Add(2*time.Hour)); len(exp) != 1 {
		t.Errorf("expired after retention = %+v", exp)
	}
	if _, err := db.GetPipelineArtifact(ctx, "a1"); !errors.Is(err, ErrPipelineArtifactNotFound) {
		t.Errorf("replaced id = %v", err)
	}
}

func TestPipelineWebhookDeliveryIdempotency(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	seedPipelineRun(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	d := PipelineWebhookDelivery{ID: "d1", PipelineID: "p1", ReceivedAt: now, Outcome: WebhookOutcomeReceived, IdempotencyKey: "k"}
	if _, err := db.AddPipelineWebhookDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishPipelineWebhookDelivery(ctx, "d1", WebhookOutcomeStarted, "", "r1", 202); err != nil {
		t.Fatal(err)
	}
	d.ID = "d2"
	first, err := db.AddPipelineWebhookDelivery(ctx, d)
	if !errors.Is(err, ErrPipelineDeliveryDuplicate) || first.ID != "d1" || first.RunID != "r1" {
		t.Fatalf("duplicate = %+v, %v", first, err)
	}
	d.ID, d.IdempotencyKey = "d3", ""
	for _, id := range []string{"d3", "d4"} {
		d.ID = id
		if _, err := db.AddPipelineWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("deliveries without a key must never collide: %v", err)
		}
	}
	if err := db.PrunePipelineWebhookDeliveries(ctx, "p1", 2); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.ListPipelineWebhookDeliveries(ctx, "p1", 10); len(list) != 2 {
		t.Errorf("after prune = %d, want 2", len(list))
	}
	if _, err := db.GetPipelineWebhookDelivery(ctx, "p1", "nope"); !errors.Is(err, ErrPipelineDeliveryNotFound) {
		t.Errorf("missing delivery = %v", err)
	}
}
