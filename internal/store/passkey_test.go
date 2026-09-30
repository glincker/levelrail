package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedUserForPasskeys(t *testing.T, db *DB, id, email string) string {
	t.Helper()
	u := User{ID: id, Email: email, DisplayName: email, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return u.ID
}

func TestSavePasskeyCredential_ThenListForUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForPasskeys(t, db, "user_1", "a@example.com")

	c := PasskeyCredential{
		ID:           "pk_1",
		UserID:       userID,
		CredentialID: "cred-id-1",
		PublicKey:    []byte{1, 2, 3},
		SignCount:    0,
		AAGUID:       "aaguid-1",
		Transports:   []string{"internal", "hybrid"},
		Label:        "MacBook Touch ID",
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.SavePasskeyCredential(ctx, c); err != nil {
		t.Fatalf("SavePasskeyCredential() error = %v", err)
	}

	rows, err := db.ListPasskeyCredentialsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListPasskeyCredentialsForUser() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.Label != c.Label || got.CredentialID != c.CredentialID || got.AAGUID != c.AAGUID {
		t.Errorf("row = %+v, want label/credential_id/aaguid matching %+v", got, c)
	}
	if len(got.Transports) != 2 || got.Transports[0] != "internal" || got.Transports[1] != "hybrid" {
		t.Errorf("Transports = %v, want [internal hybrid]", got.Transports)
	}
	if got.LastUsedAt != nil {
		t.Errorf("LastUsedAt = %v, want nil before any login", got.LastUsedAt)
	}
}

func TestSavePasskeyCredential_DuplicateCredentialIDRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userA := seedUserForPasskeys(t, db, "user_a", "a@example.com")
	userB := seedUserForPasskeys(t, db, "user_b", "b@example.com")

	first := PasskeyCredential{ID: "pk_1", UserID: userA, CredentialID: "shared-cred-id", PublicKey: []byte{1}, Label: "First", CreatedAt: time.Now().UTC()} //nolint:gosec // test-fixture credential ID, not a real secret
	if err := db.SavePasskeyCredential(ctx, first); err != nil {
		t.Fatalf("SavePasskeyCredential() error = %v", err)
	}

	// Same authenticator credential ID, different account: the unique
	// index must reject this regardless of which user is attempting it,
	// the same physical authenticator can't silently end up linked to
	// two accounts.
	dupe := PasskeyCredential{ID: "pk_2", UserID: userB, CredentialID: "shared-cred-id", PublicKey: []byte{2}, Label: "Second", CreatedAt: time.Now().UTC()} //nolint:gosec // test-fixture credential ID, not a real secret
	err := db.SavePasskeyCredential(ctx, dupe)
	if !errors.Is(err, ErrPasskeyCredentialAlreadyRegistered) {
		t.Fatalf("SavePasskeyCredential() error = %v, want ErrPasskeyCredentialAlreadyRegistered", err)
	}
}

func TestUpdatePasskeySignCountByCredentialID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForPasskeys(t, db, "user_1", "a@example.com")
	c := PasskeyCredential{ID: "pk_1", UserID: userID, CredentialID: "cred-id-1", PublicKey: []byte{1}, Label: "Key", CreatedAt: time.Now().UTC()}
	if err := db.SavePasskeyCredential(ctx, c); err != nil {
		t.Fatalf("SavePasskeyCredential() error = %v", err)
	}

	usedAt := time.Now().UTC()
	if err := db.UpdatePasskeySignCountByCredentialID(ctx, "cred-id-1", 7, usedAt); err != nil {
		t.Fatalf("UpdatePasskeySignCountByCredentialID() error = %v", err)
	}

	rows, err := db.ListPasskeyCredentialsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListPasskeyCredentialsForUser() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].SignCount != 7 {
		t.Errorf("SignCount = %d, want 7", rows[0].SignCount)
	}
	if rows[0].LastUsedAt == nil {
		t.Fatal("LastUsedAt = nil, want set after a login")
	}
}

func TestUpdatePasskeySignCountByCredentialID_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdatePasskeySignCountByCredentialID(context.Background(), "does-not-exist", 1, time.Now())
	if !errors.Is(err, ErrPasskeyCredentialNotFound) {
		t.Fatalf("error = %v, want ErrPasskeyCredentialNotFound", err)
	}
}

func TestDeletePasskeyCredential_ScopedToOwner(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userA := seedUserForPasskeys(t, db, "user_a", "a@example.com")
	userB := seedUserForPasskeys(t, db, "user_b", "b@example.com")

	c := PasskeyCredential{ID: "pk_1", UserID: userA, CredentialID: "cred-id-1", PublicKey: []byte{1}, Label: "Key", CreatedAt: time.Now().UTC()}
	if err := db.SavePasskeyCredential(ctx, c); err != nil {
		t.Fatalf("SavePasskeyCredential() error = %v", err)
	}

	// userB deleting userA's credential must not succeed: the WHERE
	// clause scopes by (id, user_id) together, a mismatched owner is
	// indistinguishable from a nonexistent row.
	if err := db.DeletePasskeyCredential(ctx, "pk_1", userB); !errors.Is(err, ErrPasskeyCredentialNotFound) {
		t.Fatalf("delete as wrong owner: error = %v, want ErrPasskeyCredentialNotFound", err)
	}
	rows, err := db.ListPasskeyCredentialsForUser(ctx, userA)
	if err != nil {
		t.Fatalf("ListPasskeyCredentialsForUser() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("credential was deleted by the wrong owner: len(rows) = %d, want 1", len(rows))
	}

	if err := db.DeletePasskeyCredential(ctx, "pk_1", userA); err != nil {
		t.Fatalf("delete as real owner: error = %v", err)
	}
	rows, err = db.ListPasskeyCredentialsForUser(ctx, userA)
	if err != nil {
		t.Fatalf("ListPasskeyCredentialsForUser() error = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) after delete = %d, want 0", len(rows))
	}
}
