package store

import (
	"errors"
	"testing"
	"time"
)

func TestCIStackTrust_Lifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestStack(t, db, "shop-prod")
	b := newTestStack(t, db, "blog-prod")

	id, err := NewCIStackTrustID()
	if err != nil {
		t.Fatal(err)
	}
	tr := CIStackTrust{ID: id, StackID: a.ID, Repository: "acme/shop", RefPattern: "refs/heads/main", Environment: "production", Verbs: []string{StackVerbApply}, CreatedBy: "root", CreatedAt: time.Now()}
	if err := db.SaveCIStackTrust(ctx, tr); err != nil {
		t.Fatal(err)
	}
	dup := tr
	dup.ID = id + "x"
	if err := db.SaveCIStackTrust(ctx, dup); !errors.Is(err, ErrCIStackTrustExists) {
		t.Fatalf("duplicate = %v", err)
	}
	bad := tr
	bad.ID, bad.Verbs = "cst_bad", []string{"destroy"}
	if err := db.SaveCIStackTrust(ctx, bad); !errors.Is(err, ErrInvalidCIStackTrust) {
		t.Fatalf("unknown verb = %v", err)
	}

	got, err := db.ListCIStackTrustsForRepo(ctx, "acme/shop", a.ID)
	if err != nil || len(got) != 1 || got[0].Verbs[0] != StackVerbApply || got[0].Environment != "production" {
		t.Fatalf("for repo = %+v, %v", got, err)
	}
	if other, _ := db.ListCIStackTrusts(ctx, b.ID); len(other) != 0 {
		t.Fatalf("trust leaked to another stack: %+v", other)
	}
	if _, err := db.DeleteCIStackTrust(ctx, b.ID, id); !errors.Is(err, ErrCIStackTrustNotFound) {
		t.Fatalf("delete via other stack = %v", err)
	}
	if _, err := db.DeleteCIStackTrust(ctx, a.ID, id); err != nil {
		t.Fatal(err)
	}
	if left, _ := db.ListCIStackTrusts(ctx, a.ID); len(left) != 0 {
		t.Fatalf("left = %+v", left)
	}
}

func TestCIStackTrust_CorruptVerbsFailClosed(t *testing.T) {
	db := openTestDB(t)
	a := newTestStack(t, db, "shop-prod")
	_, err := db.ExecContext(t.Context(), `INSERT INTO ci_stack_trusts (id, stack_id, repository, ref_pattern, verbs, created_at) VALUES ('cst_x', ?, 'acme/shop', 'refs/heads/main', 'plan,destroy', '2026-10-25T00:00:00Z')`, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ListCIStackTrusts(t.Context(), a.ID); err == nil {
		t.Fatal("corrupt verbs listed without error")
	}
}
