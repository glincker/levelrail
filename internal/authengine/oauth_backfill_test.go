package authengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/glincker/theauth-go/v2/crypto"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func (f *fixture) addIdentities(t *testing.T) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, i := range []store.OAuthIdentity{
		{ID: "oid_1", UserID: "user_aaaa", Provider: "google", ProviderUserID: "g-100", CreatedAt: now},
		{ID: "oid_2", UserID: "user_bbbb", Provider: "github", ProviderUserID: "4242", CreatedAt: now},
		{ID: "oid_3", UserID: "user_bbbb", Provider: "oidc", ProviderUserID: "sub-77", CreatedAt: now},
	} {
		if err := f.db.SaveOAuthIdentity(context.Background(), i); err != nil {
			t.Fatalf("SaveOAuthIdentity: %v", err)
		}
	}
}

func TestOAuthIdentityBackfill(t *testing.T) {
	ctx := context.Background()
	t.Run("dry run counts and writes nothing", func(t *testing.T) {
		f := newFixture(t)
		f.addIdentities(t)
		rep, err := authengine.Backfill(ctx, f.db.DB, f.opts(true))
		if err != nil || rep.OAuthIdentities != 3 {
			t.Fatalf("report %+v err %v, want 3 identities", rep, err)
		}
		if n := f.count(t, "theauth_oauth_accounts"); n != 0 {
			t.Fatalf("dry run left %d accounts", n)
		}
	})
	t.Run("copy is linked to the same user, idempotent, tokens decrypt", func(t *testing.T) {
		f := newFixture(t)
		f.addIdentities(t)
		if _, err := authengine.Backfill(ctx, f.db.DB, f.opts(false)); err != nil {
			t.Fatal(err)
		}
		rep, err := authengine.Backfill(ctx, f.db.DB, f.opts(false))
		if err != nil || rep.OAuthIdentities != 0 {
			t.Fatalf("rerun %+v err %v, want 0 new identities", rep, err)
		}
		rows, err := f.db.QueryContext(ctx, `
			SELECT a.provider, a.provider_user_id, m.legacy_id, a.access_token_enc
			FROM theauth_oauth_accounts a JOIN authengine_user_map m ON m.engine_id = a.user_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		want := map[string]string{"google/g-100": "user_aaaa", "github/4242": "user_bbbb", "oidc/sub-77": "user_bbbb"}
		got := 0
		for rows.Next() {
			var provider, puid, legacy string
			var enc []byte
			if err := rows.Scan(&provider, &puid, &legacy, &enc); err != nil {
				t.Fatal(err)
			}
			if want[provider+"/"+puid] != legacy {
				t.Errorf("%s/%s linked to %s, want %s", provider, puid, legacy, want[provider+"/"+puid])
			}
			if plain, err := crypto.Decrypt(f.key, enc); err != nil || len(plain) != 0 {
				t.Errorf("placeholder token not decryptable with the engine key: %v", err)
			}
			got++
		}
		if got != 3 {
			t.Fatalf("copied %d accounts, want 3", got)
		}
	})
	t.Run("identities without a key fail and roll back", func(t *testing.T) {
		f := newFixture(t)
		f.addIdentities(t)
		opts := f.opts(false)
		opts.EncryptionKey = nil
		if _, err := authengine.Backfill(ctx, f.db.DB, opts); err == nil {
			t.Fatal("want an error when identities exist without a key")
		}
		if f.count(t, "authengine_user_map") != 0 {
			t.Fatal("failed run was not rolled back")
		}
	})
}

func TestLinkOAuthIdentityMirrorsForMappedUsersOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rep, err := authengine.Backfill(ctx, f.db.DB, f.opts(false))
	if err != nil || rep.Users != 2 {
		t.Fatalf("backfill %+v err %v", rep, err)
	}
	eng, err := authengine.New(f.db.DB, authengine.Config{
		BaseURL: "http://app.test", TokenPrefix: "tk", TOTPIssuer: "test", EncryptionKey: f.key,
		Directory: authengine.NewDirectory(f.db.DB),
		OAuth:     &authengine.OAuthWiring{Settings: f.db, Secrets: f.mgr, Users: f.db},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Close)
	if !eng.OAuthEnabled() {
		t.Fatal("oauth wiring did not enable")
	}
	if err := eng.LinkOAuthIdentity(ctx, "user_bbbb", "google", "g-9"); err != nil {
		t.Fatal(err)
	}
	if err := eng.LinkOAuthIdentity(ctx, "user_missing", "google", "g-10"); err != nil {
		t.Fatalf("unmapped user must be skipped, got %v", err)
	}
	if err := eng.LinkOAuthIdentity(ctx, "user_bbbb", "google", "g-9"); err != nil {
		t.Fatalf("link must be idempotent: %v", err)
	}
	if n := f.count(t, "theauth_oauth_accounts"); n != 1 {
		t.Fatalf("accounts = %d, want 1", n)
	}
}

func TestOAuthStaysOffWithoutKeyOrWiring(t *testing.T) {
	f := newFixture(t)
	cfg := authengine.Config{BaseURL: "http://app.test", TokenPrefix: "tk", Directory: authengine.NewDirectory(f.db.DB)}
	eng, err := authengine.New(f.db.DB, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Close)
	if eng.OAuthEnabled() {
		t.Fatal("oauth must be off without wiring")
	}
	cfg.OAuth = &authengine.OAuthWiring{Settings: f.db, Secrets: f.mgr, Users: f.db}
	eng2, err := authengine.New(f.db.DB, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng2.Close)
	if eng2.OAuthEnabled() {
		t.Fatal("oauth must be off without an encryption key")
	}
}
