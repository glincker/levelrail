package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedTestUser(t *testing.T, db *DB, id, email string) User {
	t.Helper()
	u := User{ID: id, Email: email, DisplayName: email, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("seed user %q: %v", id, err)
	}
	return u
}

func TestSaveAndGetOAuthIdentity_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_1", "a@example.com")

	identity := OAuthIdentity{
		ID:             "oid_1",
		UserID:         u.ID,
		Provider:       OAuthProviderGoogle,
		ProviderUserID: "google-sub-123",
		CreatedAt:      time.Now().UTC(),
	}
	if err := db.SaveOAuthIdentity(ctx, identity); err != nil {
		t.Fatalf("SaveOAuthIdentity() error = %v", err)
	}

	got, err := db.GetOAuthIdentity(ctx, OAuthProviderGoogle, "google-sub-123")
	if err != nil {
		t.Fatalf("GetOAuthIdentity() error = %v", err)
	}
	if got.UserID != u.ID {
		t.Errorf("GetOAuthIdentity().UserID = %q, want %q", got.UserID, u.ID)
	}
}

func TestGetOAuthIdentity_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetOAuthIdentity(ctx, OAuthProviderGoogle, "nope"); !errors.Is(err, ErrOAuthIdentityNotFound) {
		t.Errorf("GetOAuthIdentity() error = %v, want ErrOAuthIdentityNotFound", err)
	}
}

// TestSaveOAuthIdentity_SameExternalAccountTwoUsers_Rejected is the
// account-takeover guard itself: the same external (provider,
// provider_user_id) pair must never end up linked to two different
// users, regardless of what either user's email is.
func TestSaveOAuthIdentity_SameExternalAccountTwoUsers_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u1 := seedTestUser(t, db, "user_1", "a@example.com")
	u2 := seedTestUser(t, db, "user_2", "b@example.com")

	first := OAuthIdentity{ID: "oid_1", UserID: u1.ID, Provider: OAuthProviderGitHub, ProviderUserID: "gh-42", CreatedAt: time.Now().UTC()}
	if err := db.SaveOAuthIdentity(ctx, first); err != nil {
		t.Fatalf("first SaveOAuthIdentity() error = %v", err)
	}

	second := OAuthIdentity{ID: "oid_2", UserID: u2.ID, Provider: OAuthProviderGitHub, ProviderUserID: "gh-42", CreatedAt: time.Now().UTC()}
	if err := db.SaveOAuthIdentity(ctx, second); !errors.Is(err, ErrOAuthIdentityAlreadyLinked) {
		t.Errorf("second SaveOAuthIdentity() error = %v, want ErrOAuthIdentityAlreadyLinked", err)
	}

	// The rejected link must not have moved ownership: the identity still
	// resolves to the original user.
	got, err := db.GetOAuthIdentity(ctx, OAuthProviderGitHub, "gh-42")
	if err != nil {
		t.Fatalf("GetOAuthIdentity() error = %v", err)
	}
	if got.UserID != u1.ID {
		t.Errorf("GetOAuthIdentity().UserID = %q, want unchanged owner %q", got.UserID, u1.ID)
	}
}

func TestSaveOAuthIdentity_SameUserSameProviderTwice_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_1", "a@example.com")

	first := OAuthIdentity{ID: "oid_1", UserID: u.ID, Provider: OAuthProviderGoogle, ProviderUserID: "google-1", CreatedAt: time.Now().UTC()}
	if err := db.SaveOAuthIdentity(ctx, first); err != nil {
		t.Fatalf("first SaveOAuthIdentity() error = %v", err)
	}

	second := OAuthIdentity{ID: "oid_2", UserID: u.ID, Provider: OAuthProviderGoogle, ProviderUserID: "google-2", CreatedAt: time.Now().UTC()}
	if err := db.SaveOAuthIdentity(ctx, second); !errors.Is(err, ErrOAuthIdentityAlreadyLinked) {
		t.Errorf("second SaveOAuthIdentity() error = %v, want ErrOAuthIdentityAlreadyLinked", err)
	}
}

func TestListOAuthIdentitiesForUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_1", "a@example.com")

	if err := db.SaveOAuthIdentity(ctx, OAuthIdentity{ID: "oid_1", UserID: u.ID, Provider: OAuthProviderGoogle, ProviderUserID: "g1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveOAuthIdentity(google) error = %v", err)
	}
	if err := db.SaveOAuthIdentity(ctx, OAuthIdentity{ID: "oid_2", UserID: u.ID, Provider: OAuthProviderGitHub, ProviderUserID: "h1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveOAuthIdentity(github) error = %v", err)
	}

	got, err := db.ListOAuthIdentitiesForUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListOAuthIdentitiesForUser() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ListOAuthIdentitiesForUser()) = %d, want 2", len(got))
	}
}

// TestDeleteUser_CascadesOAuthIdentities proves the FK ON DELETE CASCADE
// (migrations/0030): removing a user must not orphan their linked
// identities.
func TestDeleteUser_CascadesOAuthIdentities(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_1", "a@example.com")

	if err := db.SaveOAuthIdentity(ctx, OAuthIdentity{ID: "oid_1", UserID: u.ID, Provider: OAuthProviderGoogle, ProviderUserID: "g1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SaveOAuthIdentity() error = %v", err)
	}

	if err := db.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	if _, err := db.GetOAuthIdentity(ctx, OAuthProviderGoogle, "g1"); !errors.Is(err, ErrOAuthIdentityNotFound) {
		t.Errorf("GetOAuthIdentity() after user deletion error = %v, want ErrOAuthIdentityNotFound", err)
	}
}
