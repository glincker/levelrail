package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testToken(id, hash string) APIToken {
	return APIToken{
		ID:        id,
		Name:      "test token",
		TokenHash: hash,
		Abilities: []string{"read", "deploy"},
		CreatedAt: time.Now(),
	}
}

func TestSaveAndGetAPITokenByHash(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testToken("tok_1", "hash-abc")
	if err := db.SaveAPIToken(ctx, want); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}

	got, err := db.GetAPITokenByHash(ctx, "hash-abc")
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.TokenHash != want.TokenHash {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(got.Abilities) != 2 || got.Abilities[0] != "read" || got.Abilities[1] != "deploy" {
		t.Errorf("got.Abilities = %v, want [read deploy]", got.Abilities)
	}
	if got.RevokedAt != nil {
		t.Errorf("got.RevokedAt = %v, want nil for a freshly created token", got.RevokedAt)
	}
	if got.LastUsedAt != nil {
		t.Errorf("got.LastUsedAt = %v, want nil before any use", got.LastUsedAt)
	}
}

func TestGetAPITokenByHash_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetAPITokenByHash(context.Background(), "nonexistent"); !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("error = %v, want ErrAPITokenNotFound", err)
	}
}

func TestSaveAPIToken_WithExpiry(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	expires := time.Now().Add(24 * time.Hour)
	want := testToken("tok_exp", "hash-exp")
	want.ExpiresAt = &expires
	if err := db.SaveAPIToken(ctx, want); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}

	got, err := db.GetAPITokenByHash(ctx, "hash-exp")
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("got.ExpiresAt = nil, want a value")
	}
	if !got.ExpiresAt.Equal(expires) {
		t.Errorf("got.ExpiresAt = %v, want %v", got.ExpiresAt, expires)
	}
}

func TestListAPITokens_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := testToken("tok_a", "hash-a")
	first.CreatedAt = time.Now().Add(-1 * time.Hour)
	second := testToken("tok_b", "hash-b")
	second.CreatedAt = time.Now()

	if err := db.SaveAPIToken(ctx, first); err != nil {
		t.Fatalf("SaveAPIToken(first) error = %v", err)
	}
	if err := db.SaveAPIToken(ctx, second); err != nil {
		t.Fatalf("SaveAPIToken(second) error = %v", err)
	}

	got, err := db.ListAPITokens(ctx)
	if err != nil {
		t.Fatalf("ListAPITokens() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != "tok_b" || got[1].ID != "tok_a" {
		t.Errorf("order = [%s %s], want [tok_b tok_a] (newest first)", got[0].ID, got[1].ID)
	}
}

func TestListAPITokens_IncludesRevoked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tok := testToken("tok_r", "hash-r")
	if err := db.SaveAPIToken(ctx, tok); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}
	if err := db.RevokeAPIToken(ctx, "tok_r"); err != nil {
		t.Fatalf("RevokeAPIToken() error = %v", err)
	}

	got, err := db.ListAPITokens(ctx)
	if err != nil {
		t.Fatalf("ListAPITokens() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (revoked tokens stay in the list)", len(got))
	}
	if got[0].RevokedAt == nil {
		t.Error("got[0].RevokedAt = nil, want a timestamp after revocation")
	}
}

func TestRevokeAPIToken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveAPIToken(ctx, testToken("tok_rev", "hash-rev")); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}

	if err := db.RevokeAPIToken(ctx, "tok_rev"); err != nil {
		t.Fatalf("RevokeAPIToken() error = %v", err)
	}

	got, err := db.GetAPITokenByHash(ctx, "hash-rev")
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("got.RevokedAt = nil, want a timestamp after RevokeAPIToken")
	}
}

func TestRevokeAPIToken_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveAPIToken(ctx, testToken("tok_idem", "hash-idem")); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}
	if err := db.RevokeAPIToken(ctx, "tok_idem"); err != nil {
		t.Fatalf("first RevokeAPIToken() error = %v", err)
	}
	if err := db.RevokeAPIToken(ctx, "tok_idem"); err != nil {
		t.Errorf("second RevokeAPIToken() error = %v, want nil (idempotent)", err)
	}
}

func TestRevokeAPIToken_NotFound(t *testing.T) {
	db := openTestDB(t)
	if err := db.RevokeAPIToken(context.Background(), "ghost"); !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("error = %v, want ErrAPITokenNotFound", err)
	}
}

func TestTouchAPITokenLastUsed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveAPIToken(ctx, testToken("tok_touch", "hash-touch")); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}
	if err := db.TouchAPITokenLastUsed(ctx, "tok_touch"); err != nil {
		t.Fatalf("TouchAPITokenLastUsed() error = %v", err)
	}

	got, err := db.GetAPITokenByHash(ctx, "hash-touch")
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if got.LastUsedAt == nil {
		t.Error("got.LastUsedAt = nil, want a timestamp after TouchAPITokenLastUsed")
	}
}
