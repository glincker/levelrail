package store

import (
	"context"
	"strings"
	"testing"
)

func TestInstanceFingerprint_StableAndKeyed(t *testing.T) {
	ctx := context.Background()
	a, b := openTestDB(t), openTestDB(t)
	ia, err := a.GetInstanceInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := a.GetInstanceInfo(ctx)
	ib, _ := b.GetInstanceInfo(ctx)
	if ia != again {
		t.Errorf("fingerprint changed between calls: %+v vs %+v", ia, again)
	}
	if ia.Fingerprint == ib.Fingerprint || ia.Fingerprint == "" || strings.Contains(ia.Fingerprint, ia.InstanceID) {
		t.Errorf("fingerprints %q and %q must differ and not embed the id", ia.Fingerprint, ib.Fingerprint)
	}
}

func TestReconcileInstanceHost(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	dir := t.TempDir()
	before, _ := db.GetInstanceInfo(ctx)

	if moved, err := db.ReconcileInstanceHost(ctx, dir); err != nil || moved {
		t.Fatalf("first boot: moved=%v err=%v", moved, err)
	}
	if moved, err := db.ReconcileInstanceHost(ctx, dir); err != nil || moved {
		t.Fatalf("same data dir: moved=%v err=%v", moved, err)
	}
	if after, _ := db.GetInstanceInfo(ctx); after.InstanceID != before.InstanceID {
		t.Fatal("identity must not change on an ordinary restart")
	}

	moved, err := db.ReconcileInstanceHost(ctx, t.TempDir())
	if err != nil || !moved {
		t.Fatalf("fresh data dir: moved=%v err=%v", moved, err)
	}
	rotated, _ := db.GetInstanceInfo(ctx)
	if rotated.InstanceID == before.InstanceID || rotated.PreviousInstanceID != before.InstanceID {
		t.Fatalf("rotated = %+v, want new id with previous %q", rotated, before.InstanceID)
	}

	if err := db.ConfirmSameInstance(ctx); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if restored, _ := db.GetInstanceInfo(ctx); restored.InstanceID != before.InstanceID || restored.Fingerprint != before.Fingerprint {
		t.Fatalf("restored = %+v, want %+v", restored, before)
	}
	if err := db.ConfirmSameInstance(ctx); err != ErrNoPreviousInstance {
		t.Fatalf("second confirm err = %v, want ErrNoPreviousInstance", err)
	}
}

func TestBindOrCheckToken(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if ok, err := db.BindOrCheckToken(ctx, "tok", "inst_a", "1.1.1.1"); err != nil || !ok {
		t.Fatalf("first use: ok=%v err=%v", ok, err)
	}
	if ok, _ := db.BindOrCheckToken(ctx, "tok", "inst_a", "2.2.2.2"); !ok {
		t.Fatal("same instance must stay allowed")
	}
	if ok, _ := db.BindOrCheckToken(ctx, "tok", "inst_b", "1.1.1.1"); ok {
		t.Fatal("different instance must be refused")
	}
	bindings, _ := db.ListTokenBindings(ctx)
	if bindings["tok"].InstanceID != "inst_a" || bindings["tok"].LastIP != "2.2.2.2" {
		t.Fatalf("binding = %+v", bindings["tok"])
	}
}
