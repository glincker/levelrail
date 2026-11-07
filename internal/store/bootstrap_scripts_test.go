package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBootstrapScriptCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	a := BootstrapScript{ID: "bs_a", Name: "base", Body: "apt-get install -y htop", CreatedBy: "ana", CreatedAt: now, UpdatedAt: now}
	if err := db.CreateBootstrapScript(ctx, a); err != nil {
		t.Fatal(err)
	}
	dup := a
	dup.ID = "bs_b"
	if err := db.CreateBootstrapScript(ctx, dup); !errors.Is(err, ErrBootstrapScriptNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	got, err := db.GetBootstrapScript(ctx, "bs_a")
	if err != nil || got.Body != a.Body || got.CreatedBy != "ana" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := db.UpdateBootstrapScript(ctx, "bs_a", "base2", "echo hi", now); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountBootstrapScripts(ctx); n != 1 {
		t.Fatalf("count = %d", n)
	}
	if err := db.DeleteBootstrapScript(ctx, "bs_a"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBootstrapScript(ctx, "bs_a"); !errors.Is(err, ErrBootstrapScriptNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
	if _, err := db.GetBootstrapScript(ctx, "bs_a"); !errors.Is(err, ErrBootstrapScriptNotFound) {
		t.Fatalf("get after delete err = %v", err)
	}
}

func TestBootstrapScriptLimitsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if l, err := db.GetBootstrapScriptLimits(ctx); err != nil || l.MaxBytes != nil || l.MaxCount != nil {
		t.Fatalf("fresh = %+v, %v", l, err)
	}
	n := int64(10)
	if err := db.SetBootstrapScriptLimits(ctx, BootstrapScriptLimits{MaxBytes: &n}); err != nil {
		t.Fatal(err)
	}
	if l, _ := db.GetBootstrapScriptLimits(ctx); l.MaxBytes == nil || *l.MaxBytes != 10 || l.MaxCount != nil {
		t.Fatalf("got %+v", l)
	}
}
