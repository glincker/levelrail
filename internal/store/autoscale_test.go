package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
)

func TestAutoscaleRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAutoscalePolicy(ctx, "web"); !errors.Is(err, autoscale.ErrNotFound) {
		t.Fatalf("missing policy = %v", err)
	}
	pol := autoscale.Policy{App: "web", MinReplicas: 2, MaxReplicas: 8, Metric: autoscale.MetricRequestsPerReplica, Target: 120.5}.Normalize()
	if err := db.SaveAutoscalePolicy(ctx, pol, autoscale.Event{App: "web", Kind: autoscale.EventConfigured, At: now}, now); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetAutoscalePolicy(ctx, "web")
	if err != nil || got != pol {
		t.Fatalf("policy = %+v err = %v, want %+v", got, err, pol)
	}

	st := autoscale.State{App: "web", Current: 2, Desired: 4, Value: 180, ValueKnown: true, Reason: autoscale.ReasonScaledUp,
		BlockedBy: autoscale.BlockedStepLimit, Message: "m", LastScaleUp: now, DownSince: now.Add(-time.Minute), DownFloor: 3, UpdatedAt: now}
	ev := []autoscale.Event{{App: "web", Kind: autoscale.EventScaledUp, Current: 2, Desired: 4, Value: 180, Reason: autoscale.ReasonScaledUp, At: now}}
	if err := db.SaveAutoscaleState(ctx, st, ev); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := db.GetAutoscaleState(ctx, "web")
	if err != nil || !ok || !loaded.LastScaleUp.Equal(now) || !loaded.LastScaleDown.IsZero() || loaded.DownFloor != 3 || !loaded.ValueKnown || loaded.BlockedBy != autoscale.BlockedStepLimit {
		t.Fatalf("state = %+v ok = %v err = %v", loaded, ok, err)
	}
	events, err := db.ListAutoscaleEvents(ctx, "web", 10)
	if err != nil || len(events) != 2 || events[0].Kind != autoscale.EventScaledUp || events[1].Kind != autoscale.EventConfigured {
		t.Fatalf("events = %+v err = %v", events, err)
	}

	replicas, suspended, err := db.AutoscaleReplicas(ctx, "web")
	if err != nil || replicas != 2 || suspended {
		t.Fatalf("replicas = %d suspended = %v err = %v", replicas, suspended, err)
	}
	if err := db.SetAutoscaleReplicas(ctx, "web", 4); err != nil {
		t.Fatal(err)
	}
	if replicas, _, _ = db.AutoscaleReplicas(ctx, "web"); replicas != 4 {
		t.Fatalf("replicas = %d, want 4", replicas)
	}
	appEvents, err := db.ListAppEvents(ctx, "web", nil, time.Time{}, []string{AppEventScale}, 5)
	if err != nil || len(appEvents) != 1 || appEvents[0].Actor != "autoscaler" {
		t.Fatalf("timeline = %+v err = %v", appEvents, err)
	}
	if _, _, err := db.AutoscaleReplicas(ctx, "ghost"); !errors.Is(err, autoscale.ErrNotFound) {
		t.Fatalf("unknown app = %v", err)
	}
	if err := db.SetAutoscaleReplicas(ctx, "ghost", 2); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("unknown app set = %v", err)
	}

	if err := db.DeleteAutoscalePolicy(ctx, "web", autoscale.Event{App: "web", Kind: autoscale.EventCleared, At: now}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetAutoscaleState(ctx, "web"); ok {
		t.Error("state survived the policy")
	}
	if err := db.DeleteAutoscalePolicy(ctx, "web", autoscale.Event{App: "web"}); !errors.Is(err, autoscale.ErrNotFound) {
		t.Errorf("second delete = %v", err)
	}
	if events, _ = db.ListAutoscaleEvents(ctx, "web", 10); len(events) != 3 {
		t.Errorf("history must outlive the policy, got %d events", len(events))
	}
}

func TestAutoscaleEventsArePruned(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	events := make([]autoscale.Event, autoscaleEventsKept+25)
	for i := range events {
		events[i] = autoscale.Event{App: "web", Kind: autoscale.EventHeld, At: now}
	}
	if err := db.SaveAutoscaleState(ctx, autoscale.State{App: "web", UpdatedAt: now}, events); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListAutoscaleEvents(ctx, "web", 10000)
	if err != nil || len(got) != autoscaleEventsKept {
		t.Fatalf("kept %d events, want %d (err %v)", len(got), autoscaleEventsKept, err)
	}
}
