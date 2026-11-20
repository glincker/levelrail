package authengine

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	theauth "github.com/glincker/theauth-go/v2"
	_ "modernc.org/sqlite"

	"github.com/GLINCKER/levelrail/internal/store"
)

func linkRuntime(t *testing.T) *oauthRuntime {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`CREATE TABLE users (id TEXT, email TEXT, abilities TEXT)`,
		`CREATE TABLE authengine_user_map (legacy_id TEXT, engine_id TEXT)`,
		`CREATE TABLE theauth_oauth_accounts (provider TEXT, provider_user_id TEXT)`,
		`CREATE TABLE theauth_users (email TEXT)`,
		`INSERT INTO users VALUES ('u1', 'member@example.com', '["read"]'), ('r1', 'root@example.com', '["root"]')`,
		`INSERT INTO authengine_user_map VALUES ('u1', 'e1'), ('r1', 'e2')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return &oauthRuntime{db: db}
}

func TestCheckSignup_EmailLinking(t *testing.T) {
	rt := linkRuntime(t)
	const tenant = "11111111-2222-3333-4444-555555555555"
	cases := []struct {
		name     string
		provider string
		tenant   string
		email    string
		wantErr  error
	}{
		{"microsoft empty tenant does not link", store.OAuthProviderMicrosoft, "", "member@example.com", errEmailInUse},
		{"microsoft common tenant does not link", store.OAuthProviderMicrosoft, "common", "member@example.com", errEmailInUse},
		{"microsoft tenant set links", store.OAuthProviderMicrosoft, tenant, "member@example.com", nil},
		{"root never links with tenant", store.OAuthProviderMicrosoft, tenant, "root@example.com", errEmailInUse},
		{"root never links on other provider", store.OAuthProviderGoogle, "", "root@example.com", errEmailInUse},
		{"other provider links non root", store.OAuthProviderGoogle, "", "member@example.com", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &theauth.ProviderUser{ID: "sub-" + tc.name, Email: tc.email, EmailVerified: true}
			err := rt.checkSignup(context.Background(), tc.provider, store.OAuthProviderSettings{TenantID: tc.tenant}, info)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}
