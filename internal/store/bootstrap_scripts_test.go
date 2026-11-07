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
