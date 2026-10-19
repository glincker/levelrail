package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUserPreferences_DefaultsAndRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_p", "p@example.com")

	got, err := db.GetUserPreferences(ctx, u.ID)
	if err != nil || got.Gravatar || got.Timezone != "" {
		t.Fatalf("defaults = %+v (%v), want all empty and gravatar off", got, err)
	}
	want := UserPreferences{UserID: u.ID, Timezone: "Europe/Berlin", Language: "de", Theme: "dark", LandingPage: "/apps", Gravatar: true}
	if err := db.SetUserPreferences(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetUserPreferences(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.UpdatedAt = time.Time{}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEmailChange_TokenRules(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	owner := seedTestUser(t, db, "user_e1", "e1@example.com")
	other := seedTestUser(t, db, "user_e2", "e2@example.com")
	now := time.Now()
	mk := func(id, hash string, expires time.Time) {
		t.Helper()
		err := db.CreateEmailChangeRequest(ctx, EmailChangeRequest{ID: id, UserID: owner.ID, NewEmail: "new@example.com", TokenHash: hash, CreatedAt: now, ExpiresAt: expires})
		if err != nil {
			t.Fatal(err)
		}
	}

	mk("ec_1", "hash-live", now.Add(time.Hour))
	if _, err := db.ConsumeEmailChange(ctx, "hash-live", other.ID, now); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Fatalf("another user's consume = %v, want invalid", err)
	}
	if got, err := db.ConsumeEmailChange(ctx, "hash-live", owner.ID, now); err != nil || got != "new@example.com" {
		t.Fatalf("owner consume = %q (%v)", got, err)
	}
	if _, err := db.ConsumeEmailChange(ctx, "hash-live", owner.ID, now); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Fatalf("reuse = %v, want invalid", err)
	}

	mk("ec_2", "hash-old", now.Add(-time.Minute))
	if _, err := db.ConsumeEmailChange(ctx, "hash-old", owner.ID, now); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Fatalf("expired = %v, want invalid", err)
	}

	mk("ec_3", "hash-first", now.Add(time.Hour))
	mk("ec_4", "hash-second", now.Add(time.Hour))
	if _, err := db.ConsumeEmailChange(ctx, "hash-first", owner.ID, now); !errors.Is(err, ErrEmailChangeInvalid) {
		t.Fatalf("superseded request = %v, want invalid", err)
	}
}

func TestConsumeEmailChange_TakenAddressKeepsToken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	owner := seedTestUser(t, db, "user_c1", "c1@example.com")
	seedTestUser(t, db, "user_c2", "taken@example.com")
	now := time.Now()
	err := db.CreateEmailChangeRequest(ctx, EmailChangeRequest{ID: "ec_c", UserID: owner.ID, NewEmail: "taken@example.com", TokenHash: "h", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ConsumeEmailChange(ctx, "h", owner.ID, now); !errors.Is(err, ErrUserEmailExists) {
		t.Fatalf("taken address = %v, want ErrUserEmailExists", err)
	}
	var used string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(used_at, '') FROM email_change_requests WHERE id = 'ec_c'`).Scan(&used); err != nil || used != "" {
		t.Fatalf("token burned on conflict: used_at=%q (%v)", used, err)
	}
}

func TestUpdateUserEmail(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	a := seedTestUser(t, db, "user_a", "a@example.com")
	seedTestUser(t, db, "user_b", "b@example.com")

	if err := db.UpdateUserEmail(ctx, a.ID, "B@Example.com"); !errors.Is(err, ErrUserEmailExists) {
		t.Fatalf("taken address = %v, want ErrUserEmailExists", err)
	}
	if err := db.UpdateUserEmail(ctx, a.ID, "  New@Example.com "); err != nil {
		t.Fatal(err)
	}
	if got, err := db.GetUserByID(ctx, a.ID); err != nil || got.Email != "new@example.com" {
		t.Fatalf("email = %q (%v)", got.Email, err)
	}
	if err := db.UpdateUserEmail(ctx, "nobody", "x@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user = %v, want ErrUserNotFound", err)
	}
}

func TestDeleteOAuthIdentityForUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_o", "o@example.com")
	v := seedTestUser(t, db, "user_o2", "o2@example.com")
	for _, i := range []OAuthIdentity{
		{ID: "o1", UserID: u.ID, Provider: OAuthProviderGitHub, ProviderUserID: "gh-1", CreatedAt: time.Now()},
		{ID: "o2", UserID: v.ID, Provider: OAuthProviderGitHub, ProviderUserID: "gh-2", CreatedAt: time.Now()},
	} {
		if err := db.SaveOAuthIdentity(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DeleteOAuthIdentityForUser(ctx, u.ID, OAuthProviderGitHub); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOAuthIdentityForUser(ctx, u.ID, OAuthProviderGitHub); !errors.Is(err, ErrOAuthIdentityNotFound) {
		t.Fatalf("second delete = %v, want not found", err)
	}
	if ids, _ := db.ListOAuthIdentitiesForUser(ctx, v.ID); len(ids) != 1 {
		t.Fatalf("another user's identity count = %d, want 1", len(ids))
	}
}

func TestListAuditEntries_OwnerFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, APIToken{ID: "tok_mine", Name: "mine", TokenHash: "h1", Abilities: []string{"read"}, OwnerUserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	rows := []AuditEntry{
		{ID: "a1", ActorType: "session", ActorID: "u1"},
		{ID: "a2", ActorType: "session", ActorID: "u2"},
		{ID: "a3", ActorType: "token", ActorID: "tok_mine"},
		{ID: "a4", ActorType: "token", ActorID: "tok_other"},
		{ID: "a5", ActorType: "token", ActorID: "tok_ai", OnBehalfOf: "user:u1"},
	}
	for i, r := range rows {
		r.Ability, r.Method, r.Path, r.StatusCode = "write", "POST", "/x", 200
		r.CreatedAt = FormatAuditTime(time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC))
		if err := db.SaveAuditEntry(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{OwnerUserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	if len(ids) != 3 || !ids["a1"] || !ids["a3"] || !ids["a5"] {
		t.Fatalf("owner u1 rows = %v, want a1 a3 a5", ids)
	}
	tokens, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{OwnerUserID: "u1", ActorType: "token"})
	if err != nil || len(tokens) != 2 {
		t.Fatalf("owner plus token filter = %d rows (%v), want 2", len(tokens), err)
	}
}
