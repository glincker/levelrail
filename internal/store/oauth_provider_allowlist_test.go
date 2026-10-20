package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOAuthProviderAllowlistMatchesSettingsRows(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.ListOAuthProviderSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seeded := map[string]bool{}
	for _, r := range rows {
		seeded[r.Provider] = true
		if !IsOAuthProvider(r.Provider) {
			t.Errorf("settings row %q is not in the allowlist", r.Provider)
		}
	}
	seen := map[string]bool{}
	for _, p := range OAuthProviders() {
		if seen[p] {
			t.Errorf("allowlist repeats %q", p)
		}
		seen[p] = true
		if !seeded[p] {
			t.Errorf("provider %q has no seeded settings row", p)
		}
	}
}

func TestSaveOAuthIdentity_ProviderAllowlist(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedTestUser(t, db, "user_p", "p@example.com")
	bad := OAuthIdentity{ID: "oid_bad", UserID: u.ID, Provider: "myspace", ProviderUserID: "1", CreatedAt: time.Now().UTC()}
	if err := db.SaveOAuthIdentity(ctx, bad); !errors.Is(err, ErrUnknownOAuthProvider) {
		t.Fatalf("err = %v, want ErrUnknownOAuthProvider", err)
	}
	for _, p := range OAuthProviders() {
		id := OAuthIdentity{ID: "oid_" + p, UserID: u.ID, Provider: p, ProviderUserID: "x", CreatedAt: time.Now().UTC()}
		if err := db.SaveOAuthIdentity(ctx, id); err != nil {
			t.Fatalf("provider %q: %v", p, err)
		}
	}
}
