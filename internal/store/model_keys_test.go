package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSaveModelCreatesDefaultKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatalf("SaveModel: %v", err)
	}
	keys, err := db.ListModelKeys(ctx, "chat")
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListModelKeys = %v, %v", keys, err)
	}
	if keys[0].Name != DefaultModelKeyName || keys[0].KeyHash != "h" || keys[0].ID != DefaultModelKeyID("chat") {
		t.Fatalf("default key = %+v", keys[0])
	}
}

func TestModelKeyLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatal(err)
	}
	k := ModelKey{ID: "k1", ModelName: "chat", Name: "ci", KeyHash: "h1", KeyPrefix: "lr-12345", RPM: 60, AllowPaths: []string{"/v1/chat/completions"}, CreatedAt: now}
	if err := db.CreateModelKey(ctx, k); err != nil {
		t.Fatalf("CreateModelKey: %v", err)
	}
	dup := k
	dup.ID, dup.KeyHash = "k2", "h2"
	if err := db.CreateModelKey(ctx, dup); !errors.Is(err, ErrModelKeyExists) {
		t.Fatalf("duplicate name err = %v", err)
	}
	if err := db.CreateModelKey(ctx, ModelKey{ID: "x", ModelName: "nope", Name: "a", CreatedAt: now}); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("unknown model err = %v", err)
	}

	grace := now.Add(time.Hour)
	next := ModelKey{ID: "k3", ModelName: "chat", Name: "ci", KeyHash: "h3", KeyPrefix: "lr-abcdef", RPM: 60, CreatedAt: now}
	if err := db.RotateModelKey(ctx, "chat", "k1", next, &grace, now); err != nil {
		t.Fatalf("RotateModelKey: %v", err)
	}
	old, err := db.GetModelKey(ctx, "chat", "k1")
	if err != nil || old.ReplacedBy != "k3" || old.ExpiresAt == nil || !old.ExpiresAt.Equal(grace) || old.RevokedAt != nil {
		t.Fatalf("old key = %+v, %v", old, err)
	}
	if err := db.RotateModelKey(ctx, "chat", "k1", ModelKey{ID: "k4", ModelName: "chat", Name: "ci", KeyHash: "h4", CreatedAt: now}, nil, now); !errors.Is(err, ErrModelKeyNotFound) {
		t.Fatalf("rotating a retired key err = %v", err)
	}

	if err := db.RevokeModelKey(ctx, "chat", "k3", now); err != nil {
		t.Fatalf("RevokeModelKey: %v", err)
	}
	active, err := db.ListActiveModelKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.ID == "k3" {
			t.Fatal("revoked key listed as active")
		}
	}
	if err := db.RevokeModelKey(ctx, "chat", "missing", now); !errors.Is(err, ErrModelKeyNotFound) {
		t.Fatalf("revoke missing err = %v", err)
	}
}

func TestRotateModelKeyImmediate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatal(err)
	}
	next := ModelKey{ID: "n1", ModelName: "chat", Name: DefaultModelKeyName, KeyHash: "hn", KeyPrefix: "lr-nnnnnn", CreatedAt: now}
	if err := db.RotateModelKey(ctx, "chat", DefaultModelKeyID("chat"), next, nil, now); err != nil {
		t.Fatalf("RotateModelKey: %v", err)
	}
	old, err := db.GetModelKey(ctx, "chat", DefaultModelKeyID("chat"))
	if err != nil || old.RevokedAt == nil {
		t.Fatalf("old key = %+v, %v", old, err)
	}
}

func TestRotateModelAPIKeyLegacyKeepsDefaultKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatal(err)
	}
	if err := db.RotateModelAPIKey(ctx, "chat", "newhash", "lr-newpref"); err != nil {
		t.Fatalf("RotateModelAPIKey: %v", err)
	}
	keys, err := db.ListModelKeys(ctx, "chat")
	if err != nil || len(keys) != 1 || keys[0].KeyHash != "newhash" {
		t.Fatalf("keys = %+v, %v", keys, err)
	}
	if err := db.RotateModelAPIKey(ctx, "missing", "x", "y"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("missing model err = %v", err)
	}
}

func TestModelUsageAccumulatesAndPrunes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	hour := time.Now().UTC().Truncate(time.Hour)
	u := ModelUsage{ModelName: "chat", KeyID: "k1", HourStart: hour, Requests: 2, Status2xx: 2, InputTokens: 10, OutputTokens: 5, UsageRequests: 1}
	if err := db.AddModelUsage(ctx, []ModelUsage{u}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddModelUsage(ctx, []ModelUsage{u}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListModelUsage(ctx, "chat", hour.Add(-time.Hour), hour.Add(time.Hour))
	if err != nil || len(got) != 1 || got[0].Requests != 4 || got[0].InputTokens != 20 || got[0].OutputTokens != 10 {
		t.Fatalf("usage = %+v, %v", got, err)
	}
	n, err := db.PruneModelUsage(ctx, hour.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune = %d, %v", n, err)
	}
}

func TestDeleteModelRemovesKeysAndUsage(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	if err := db.AddModelUsage(ctx, []ModelUsage{{ModelName: "chat", KeyID: "k", HourStart: hour, Requests: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteModel(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	if keys, _ := db.ListModelKeys(ctx, "chat"); len(keys) != 0 {
		t.Fatalf("keys remain: %+v", keys)
	}
	if u, _ := db.ListModelUsage(ctx, "chat", hour.Add(-time.Hour), hour.Add(time.Hour)); len(u) != 0 {
		t.Fatalf("usage remains: %+v", u)
	}
}
