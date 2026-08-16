package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetPasswordResetTokenByHash_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetPasswordResetTokenByHash(context.Background(), "nope")
	if !errors.Is(err, ErrPasswordResetTokenNotFound) {
		t.Errorf("error = %v, want ErrPasswordResetTokenNotFound", err)
	}
}

func TestSaveAndGetPasswordResetToken_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	want := PasswordResetToken{
		ID:        "prt_1",
		TokenHash: "deadbeef",
		CreatedAt: now,
		ExpiresAt: now.Add(30 * time.Minute),
	}
	if err := db.SavePasswordResetToken(ctx, want); err != nil {
		t.Fatalf("SavePasswordResetToken() error = %v", err)
	}

	got, err := db.GetPasswordResetTokenByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash() error = %v", err)
	}
	if got.ID != want.ID || got.TokenHash != want.TokenHash {
		t.Errorf("got %+v, want ID/TokenHash matching %+v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("got CreatedAt=%v ExpiresAt=%v, want %v / %v", got.CreatedAt, got.ExpiresAt, want.CreatedAt, want.ExpiresAt)
	}
	if got.UsedAt != nil {
		t.Errorf("UsedAt = %v, want nil (not used yet)", got.UsedAt)
	}
}

func TestMarkPasswordResetTokenUsed_SetsUsedAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.SavePasswordResetToken(ctx, PasswordResetToken{
		ID: "prt_1", TokenHash: "deadbeef", CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.MarkPasswordResetTokenUsed(ctx, "prt_1"); err != nil {
		t.Fatalf("MarkPasswordResetTokenUsed() error = %v", err)
	}

	got, err := db.GetPasswordResetTokenByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash() error = %v", err)
	}
	if got.UsedAt == nil {
		t.Fatal("UsedAt = nil, want set after MarkPasswordResetTokenUsed")
	}
}

func TestSavePasswordResetToken_DuplicateHashRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	tok := PasswordResetToken{ID: "prt_1", TokenHash: "same-hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.SavePasswordResetToken(ctx, tok); err != nil {
		t.Fatalf("first save: %v", err)
	}
	tok2 := tok
	tok2.ID = "prt_2"
	if err := db.SavePasswordResetToken(ctx, tok2); err == nil {
		t.Error("second SavePasswordResetToken() with a duplicate token_hash error = nil, want a UNIQUE constraint error")
	}
}
