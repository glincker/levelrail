package models

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestIdleTTLAndActivity(t *testing.T) {
	t.Setenv("APP_MODEL_IDLE_TTL", "20m")
	now := time.Now()
	tests := []struct {
		name       string
		m          store.Model
		wantTTL    time.Duration
		wantActive bool
	}{
		{"default ttl from env", store.Model{LastActiveAt: now.Add(-19 * time.Minute)}, 20 * time.Minute, true},
		{"own ttl wins", store.Model{IdleTTLSeconds: 60, LastActiveAt: now.Add(-2 * time.Minute)}, time.Minute, false},
		{"never used is idle", store.Model{}, 20 * time.Minute, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IdleTTL(tc.m); got != tc.wantTTL {
				t.Errorf("ttl = %s, want %s", got, tc.wantTTL)
			}
			if got := ResidencyActive(tc.m, now); got != tc.wantActive {
				t.Errorf("active = %v, want %v", got, tc.wantActive)
			}
		})
	}
	t.Setenv("APP_MODEL_IDLE_TTL", "0s")
	if got := DefaultIdleTTL(); got != defaultIdleTTL {
		t.Errorf("a zero env value must fall back to the default, got %s", got)
	}
}

func TestServiceResidency(t *testing.T) {
	ctx := context.Background()
	svc, db := newSvc(t, nil)

	if _, err := svc.Create(ctx, CreateInput{Spec: spec("bad"), Residency: "sometimes"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad residency err = %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("short"), Residency: store.ResidencyOnDemand, IdleTTL: 5 * time.Second}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a ttl below the floor must be rejected, got %v", err)
	}

	got, err := svc.Create(ctx, CreateInput{Spec: spec("chat"), Residency: store.ResidencyOnDemand, IdleTTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	m := got.Model
	if m.Residency != store.ResidencyOnDemand || m.IdleTTLSeconds != 600 || m.LastActiveAt.IsZero() {
		t.Fatalf("created = %+v: a new on-demand model starts active so it loads first", m)
	}

	always, err := svc.Create(ctx, CreateInput{Spec: spec("always")})
	if err != nil {
		t.Fatal(err)
	}
	if always.Model.Residency != store.ResidencyAlways || !always.Model.LastActiveAt.IsZero() {
		t.Fatalf("default residency = %+v", always.Model)
	}
	if err := svc.Wake(ctx, "always"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("waking an always-resident model = %v, want ErrInvalid", err)
	}
	if err := svc.Sleep(ctx, "always"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sleeping an always-resident model = %v, want ErrInvalid", err)
	}

	if err := svc.Sleep(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	after, _ := db.GetModel(ctx, "chat")
	if !after.LastActiveAt.IsZero() {
		t.Fatalf("sleep must clear activity: %+v", after)
	}
	if err := svc.Wake(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	after, _ = db.GetModel(ctx, "chat")
	if !ResidencyActive(*after, time.Now()) {
		t.Fatalf("wake must mark the model active: %+v", after)
	}

	if err := svc.SetResidency(ctx, "always", store.ResidencyOnDemand, 0); err != nil {
		t.Fatal(err)
	}
	flipped, _ := db.GetModel(ctx, "always")
	if flipped.Residency != store.ResidencyOnDemand || flipped.LastActiveAt.IsZero() {
		t.Fatalf("flipped = %+v", flipped)
	}
	if err := svc.SetResidency(ctx, "nope", store.ResidencyAlways, 0); !errors.Is(err, store.ErrModelNotFound) {
		t.Fatalf("missing model err = %v", err)
	}
}
