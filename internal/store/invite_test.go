package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetInviteByHash_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetInviteByHash(context.Background(), "nope")
	if !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("error = %v, want ErrInviteNotFound", err)
	}
}

func TestSaveAndGetInvite_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")

	now := time.Now().UTC().Truncate(time.Millisecond)
	want := Invite{
		ID:        "inv_1",
		Email:     "new@example.com",
		Role:      "operator",
		Abilities: []string{"read", "write", "deploy"},
		TokenHash: "deadbeef",
		CreatedBy: admin.ID,
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	if err := db.SaveInvite(ctx, want); err != nil {
		t.Fatalf("SaveInvite() error = %v", err)
	}

	got, err := db.GetInviteByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetInviteByHash() error = %v", err)
	}
	if got.ID != want.ID || got.Email != want.Email || got.Role != want.Role || got.CreatedBy != want.CreatedBy {
		t.Errorf("got %+v, want fields matching %+v", got, want)
	}
	if len(got.Abilities) != 3 {
		t.Errorf("Abilities = %v, want 3 entries", got.Abilities)
	}
	if got.AcceptedAt != nil || got.RevokedAt != nil {
		t.Errorf("AcceptedAt/RevokedAt = %v/%v, want both nil", got.AcceptedAt, got.RevokedAt)
	}

	byID, err := db.GetInviteByID(ctx, "inv_1")
	if err != nil {
		t.Fatalf("GetInviteByID() error = %v", err)
	}
	if byID.TokenHash != want.TokenHash {
		t.Errorf("GetInviteByID() TokenHash = %q, want %q", byID.TokenHash, want.TokenHash)
	}
}

func TestSaveInvite_DuplicatePendingEmailRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	first := Invite{ID: "inv_1", Email: "dup@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.SaveInvite(ctx, first); err != nil {
		t.Fatalf("first SaveInvite() error = %v", err)
	}

	second := Invite{ID: "inv_2", Email: "dup@example.com", Abilities: []string{"read"}, TokenHash: "hash2", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.SaveInvite(ctx, second); !errors.Is(err, ErrInviteEmailExists) {
		t.Errorf("second SaveInvite() error = %v, want ErrInviteEmailExists", err)
	}
}

func TestSaveInvite_SameEmailAllowedAfterRevoke(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	first := Invite{ID: "inv_1", Email: "again@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.SaveInvite(ctx, first); err != nil {
		t.Fatalf("first SaveInvite() error = %v", err)
	}
	if err := db.RevokeInvite(ctx, "inv_1"); err != nil {
		t.Fatalf("RevokeInvite() error = %v", err)
	}

	second := Invite{ID: "inv_2", Email: "again@example.com", Abilities: []string{"read"}, TokenHash: "hash2", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.SaveInvite(ctx, second); err != nil {
		t.Errorf("second SaveInvite() after revoke error = %v, want nil", err)
	}
}

func TestClaimInvite_SetsAcceptedAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	if err := db.SaveInvite(ctx, Invite{ID: "inv_1", Email: "a@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.ClaimInvite(ctx, "inv_1"); err != nil {
		t.Fatalf("ClaimInvite() error = %v", err)
	}

	got, err := db.GetInviteByID(ctx, "inv_1")
	if err != nil {
		t.Fatalf("GetInviteByID() error = %v", err)
	}
	if got.AcceptedAt == nil {
		t.Fatal("AcceptedAt = nil, want set after ClaimInvite")
	}
}

func TestClaimInvite_SecondClaimFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	if err := db.SaveInvite(ctx, Invite{ID: "inv_1", Email: "a@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.ClaimInvite(ctx, "inv_1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := db.ClaimInvite(ctx, "inv_1"); !errors.Is(err, ErrInviteAlreadyAccepted) {
		t.Fatalf("second claim error = %v, want ErrInviteAlreadyAccepted", err)
	}
}

func TestClaimInvite_AfterRevokeFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	if err := db.SaveInvite(ctx, Invite{ID: "inv_1", Email: "a@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.RevokeInvite(ctx, "inv_1"); err != nil {
		t.Fatalf("RevokeInvite() error = %v", err)
	}

	if err := db.ClaimInvite(ctx, "inv_1"); !errors.Is(err, ErrInviteAlreadyRevoked) {
		t.Fatalf("ClaimInvite() after revoke error = %v, want ErrInviteAlreadyRevoked", err)
	}
}

func TestRevokeInvite_NotFound(t *testing.T) {
	db := openTestDB(t)

	if err := db.RevokeInvite(context.Background(), "nope"); !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("RevokeInvite() error = %v, want ErrInviteNotFound", err)
	}
}

func TestRevokeInvite_AlreadyRevoked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	if err := db.SaveInvite(ctx, Invite{ID: "inv_1", Email: "a@example.com", Abilities: []string{"read"}, TokenHash: "hash1", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.RevokeInvite(ctx, "inv_1"); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := db.RevokeInvite(ctx, "inv_1"); !errors.Is(err, ErrInviteAlreadyRevoked) {
		t.Fatalf("second revoke error = %v, want ErrInviteAlreadyRevoked", err)
	}
}

func TestListPendingInvites_ExcludesAcceptedAndRevoked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	admin := seedTestUser(t, db, "user_1", "admin@example.com")
	now := time.Now().UTC()

	seed := func(id, email string) {
		t.Helper()
		if err := db.SaveInvite(ctx, Invite{ID: id, Email: email, Abilities: []string{"read"}, TokenHash: id + "-hash", CreatedBy: admin.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatalf("seed invite %q: %v", id, err)
		}
	}
	seed("inv_pending", "pending@example.com")
	seed("inv_accepted", "accepted@example.com")
	seed("inv_revoked", "revoked@example.com")

	if err := db.ClaimInvite(ctx, "inv_accepted"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := db.RevokeInvite(ctx, "inv_revoked"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	got, err := db.ListPendingInvites(ctx)
	if err != nil {
		t.Fatalf("ListPendingInvites() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "inv_pending" {
		t.Errorf("ListPendingInvites() = %+v, want exactly [inv_pending]", got)
	}
}
