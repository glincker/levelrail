package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConnectionMetaRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.GetConnectionMeta(ctx, "git:gc_1"); !errors.Is(err, ErrConnectionMetaNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	exp := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	if err := db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "git:gc_1", ScopeType: ConnectionScopeOrg, ScopeID: "org1", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "git:gc_1", ScopeType: ConnectionScopeApp, ScopeID: "web", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetConnectionMeta(ctx, "git:gc_1")
	if err != nil || got.ScopeType != ConnectionScopeApp || got.ScopeID != "web" || !got.ExpiresAt.Equal(exp) || got.Health != "unknown" || got.UpdatedAt.IsZero() {
		t.Fatalf("get = %+v, %v", got, err)
	}
	all, err := db.ListConnectionMeta(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list = %v, %v", all, err)
	}
	if err := db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "x", ScopeType: "team"}); err == nil {
		t.Fatal("expected the scope check to reject an unknown scope type")
	}
	if err := db.DeleteConnectionMeta(ctx, "git:gc_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetConnectionMeta(ctx, "git:gc_1"); !errors.Is(err, ErrConnectionMetaNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
}

func TestConnectionBindingsCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	b, err := db.CreateConnectionBinding(ctx, ConnectionBinding{Ref: "registry:rc_1", TargetType: BindingTargetApp, TargetID: "web", Role: "registry_pull"})
	if err != nil || b.ID == "" || b.Rules != "{}" {
		t.Fatalf("create = %+v, %v", b, err)
	}
	if _, err := db.CreateConnectionBinding(ctx, ConnectionBinding{Ref: "registry:rc_1", TargetType: BindingTargetApp, TargetID: "web", Role: "registry_pull"}); !errors.Is(err, ErrConnectionBindingExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := db.CreateConnectionBinding(ctx, ConnectionBinding{Ref: "r", TargetType: "node", TargetID: "n", Role: "x"}); err == nil {
		t.Fatal("expected the target type check to reject node")
	}
	byTarget, _ := db.ListConnectionBindingsByTarget(ctx, BindingTargetApp, "web")
	byRef, _ := db.ListConnectionBindingsByRef(ctx, "registry:rc_1")
	counts, _ := db.CountConnectionBindingsByRef(ctx)
	if len(byTarget) != 1 || len(byRef) != 1 || counts["registry:rc_1"] != 1 {
		t.Fatalf("lists = %v %v %v", byTarget, byRef, counts)
	}
	if err := db.DeleteConnectionBinding(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteConnectionBinding(ctx, b.ID); !errors.Is(err, ErrConnectionBindingNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
