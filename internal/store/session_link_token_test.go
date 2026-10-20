package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetSessionLinkTokenByHash_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetSessionLinkTokenByHash(context.Background(), "nope")
	if !errors.Is(err, ErrSessionLinkTokenNotFound) {
		t.Errorf("error = %v, want ErrSessionLinkTokenNotFound", err)
	}
}

func TestSaveAndGetSessionLinkToken_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	user := seedTestUser(t, db, "user_1", "user1@example.com")

	now := time.Now().UTC().Truncate(time.Millisecond)
	want := SessionLinkToken{
		ID:            "sl_1",
		PrincipalType: PrincipalTypeUser,
		PrincipalID:   user.ID,
		Abilities:     []string{"root"},
		DisplayName:   user.Email,
		TokenHash:     "deadbeef",
		CreatedAt:     now,
		ExpiresAt:     now.Add(2 * time.Minute),
	}
	if err := db.SaveSessionLinkToken(ctx, want); err != nil {
		t.Fatalf("SaveSessionLinkToken() error = %v", err)
	}

	got, err := db.GetSessionLinkTokenByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetSessionLinkTokenByHash() error = %v", err)
	}
	if got.ID != want.ID || got.PrincipalType != want.PrincipalType || got.PrincipalID != want.PrincipalID || got.TokenHash != want.TokenHash || got.DisplayName != want.DisplayName {
		t.Errorf("got %+v, want ID/PrincipalType/PrincipalID/TokenHash/DisplayName matching %+v", got, want)
	}
	if len(got.Abilities) != 1 || got.Abilities[0] != "root" {
		t.Errorf("Abilities = %v, want [root]", got.Abilities)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("got CreatedAt=%v ExpiresAt=%v, want %v / %v", got.CreatedAt, got.ExpiresAt, want.CreatedAt, want.ExpiresAt)
	}
	if got.UsedAt != nil {
		t.Errorf("UsedAt = %v, want nil (not used yet)", got.UsedAt)
	}
}

func TestClaimSessionLinkToken_SetsUsedAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	user := seedTestUser(t, db, "user_1", "user1@example.com")

	now := time.Now().UTC()
	if err := db.SaveSessionLinkToken(ctx, SessionLinkToken{
		ID: "sl_1", PrincipalType: PrincipalTypeUser, PrincipalID: user.ID, Abilities: []string{"root"},
		DisplayName: user.Email, TokenHash: "deadbeef", CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.ClaimSessionLinkToken(ctx, "sl_1"); err != nil {
		t.Fatalf("ClaimSessionLinkToken() error = %v", err)
	}

	got, err := db.GetSessionLinkTokenByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetSessionLinkTokenByHash() error = %v", err)
	}
	if got.UsedAt == nil {
		t.Fatal("UsedAt = nil, want set after ClaimSessionLinkToken")
	}
}

func TestClaimSessionLinkToken_SecondClaimFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	user := seedTestUser(t, db, "user_1", "user1@example.com")

	now := time.Now().UTC()
	if err := db.SaveSessionLinkToken(ctx, SessionLinkToken{
		ID: "sl_1", PrincipalType: PrincipalTypeUser, PrincipalID: user.ID, Abilities: []string{"root"},
		DisplayName: user.Email, TokenHash: "deadbeef", CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.ClaimSessionLinkToken(ctx, "sl_1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := db.ClaimSessionLinkToken(ctx, "sl_1"); !errors.Is(err, ErrSessionLinkTokenAlreadyUsed) {
		t.Fatalf("second claim error = %v, want ErrSessionLinkTokenAlreadyUsed", err)
	}
}

func TestSaveSessionLinkToken_DuplicateHashRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	user := seedTestUser(t, db, "user_1", "user1@example.com")
	now := time.Now().UTC()

	tok := SessionLinkToken{
		ID: "sl_1", PrincipalType: PrincipalTypeUser, PrincipalID: user.ID, Abilities: []string{"root"},
		DisplayName: user.Email, TokenHash: "same-hash", CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}
	if err := db.SaveSessionLinkToken(ctx, tok); err != nil {
		t.Fatalf("first save: %v", err)
	}
	tok2 := tok
	tok2.ID = "sl_2"
	if err := db.SaveSessionLinkToken(ctx, tok2); err == nil {
		t.Error("second SaveSessionLinkToken() with a duplicate token_hash error = nil, want a UNIQUE constraint error")
	}
}
