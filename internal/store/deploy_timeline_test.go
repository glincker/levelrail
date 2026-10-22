package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeployTimelineSaveGetPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetDeployTimeline(ctx, "missing"); !errors.Is(err, ErrDeployTimelineNotFound) {
		t.Fatalf("missing timeline error = %v", err)
	}
	frozen := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployTimeline(ctx, DeployTimelineRow{AttemptID: "dep_a", ServiceName: "web", FrozenAt: frozen, Payload: []byte(`{"v":1}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployTimeline(ctx, DeployTimelineRow{AttemptID: "dep_a", ServiceName: "web", Payload: []byte(`{"v":2}`)}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDeployTimeline(ctx, "dep_a")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) != `{"v":1}` || !got.FrozenAt.Equal(frozen) {
		t.Fatalf("second save must not overwrite the first freeze: %+v", got)
	}

	if err := db.SaveDeployTimeline(ctx, DeployTimelineRow{AttemptID: "dep_old", ServiceName: "web", FrozenAt: time.Now().Add(-48 * time.Hour), Payload: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	n, err := db.PruneDeployTimelines(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	if _, err := db.GetDeployTimeline(ctx, "dep_a"); err != nil {
		t.Fatalf("recent timeline must survive: %v", err)
	}
}

func TestDeleteDeployAttemptsRemovesTimeline(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDeployTimeline(ctx, DeployTimelineRow{AttemptID: "dep_gone", ServiceName: "web", Payload: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDeployAttempts(ctx, []string{"dep_gone"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDeployTimeline(ctx, "dep_gone"); !errors.Is(err, ErrDeployTimelineNotFound) {
		t.Fatalf("timeline must go with its attempt, err = %v", err)
	}
}
