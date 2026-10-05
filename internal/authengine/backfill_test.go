package authengine_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
	"github.com/glincker/theauth-go/v2/crypto"
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	legacyPassword = "correct horse battery"
	legacyEmail    = "Admin@Example.test"
	totpBase32     = "JBSWY3DPEHPK3PXP" //nolint:gosec // fixture value
	totpService    = "user-totp/"
	totpKey        = "secret"
)

type fixture struct {
	db        *store.DB
	mgr       *secrets.Manager
	rawTokens map[string]string
	key       []byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, mgr: secrets.NewManager(db, mk), rawTokens: map[string]string{}}
	hash, err := bcrypt.GenerateFromPassword([]byte(legacyPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := string(hash)
	now := time.Now().UTC().Truncate(time.Millisecond)
	users := []store.User{
		{ID: "user_aaaa", Email: legacyEmail, DisplayName: "Admin", PasswordHash: &h, Abilities: []string{"root"}, IsFirstUser: true, CreatedAt: now},
		{ID: "user_bbbb", Email: "dev@example.test", DisplayName: "Dev", PasswordHash: &h, Abilities: []string{"read", "read:sensitive", "deploy"}, CreatedAt: now},
	}
	for _, u := range users {
		if err := db.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
	}
	f.addToken(t, "tok_root", "user_aaaa", []string{"root"})
	f.addToken(t, "tok_read", "user_bbbb", []string{"read", "read:sensitive"})
	f.addToken(t, "tok_deploy", "user_bbbb", []string{"deploy"})
	f.addToken(t, "tok_system", "", []string{"read"})

	if err := f.mgr.SetValue(ctx, totpService+"user_aaaa", totpKey, totpBase32); err != nil {
		t.Fatal(err)
	}
	if err := db.EnableUserTOTP(ctx, "user_aaaa", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePasskeyCredential(ctx, store.PasskeyCredential{
		ID: "pk_1", UserID: "user_bbbb", CredentialID: base64.RawURLEncoding.EncodeToString([]byte("cred-1")),
		PublicKey: []byte("pubkey"), SignCount: 7, AAGUID: base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
		Transports: []string{"usb", "internal"}, Label: "laptop", BackupEligible: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	f.key = make([]byte, 32)
	if _, err := rand.Read(f.key); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) addToken(t *testing.T, id, owner string, abilities []string) {
	t.Helper()
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	err := f.db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: id, TokenHash: hex.EncodeToString(sum[:]), Abilities: abilities,
		CreatedAt: time.Now().UTC(), OwnerUserID: owner,
	})
	if err != nil {
		t.Fatalf("SaveAPIToken: %v", err)
	}
	f.rawTokens[id] = raw
}

func (f *fixture) opts(dry bool) authengine.BackfillOptions {
	return authengine.BackfillOptions{
		DryRun: dry, Secrets: f.mgr, EncryptionKey: f.key,
		TOTPSecretService: store.UserTOTPSecretsKey, TOTPSecretKey: totpKey,
	}
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *fixture) engine(t *testing.T, srv *httptest.Server) *authengine.Engine {
	t.Helper()
	eng, err := authengine.New(f.db.DB, authengine.Config{
		BaseURL: "http://" + srv.Listener.Addr().String(), PathPrefix: authengine.DefaultPathPrefix,
		TokenPrefix: "tk", EncryptionKey: f.key, TOTPIssuer: "test", RateLimitPerIP: 1000,
		Directory: authengine.NewDirectory(f.db.DB),
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	srv.Config.Handler = eng.Handler()
	srv.Start()
	t.Cleanup(srv.Close)
	return eng
}

func TestBackfillDryRunWritesNothing(t *testing.T) {
	f := newFixture(t)
	rep, err := authengine.Backfill(context.Background(), f.db.DB, f.opts(true))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if rep.Users != 2 || rep.Tokens != 3 || rep.TokensSkippedNoOwner != 1 || rep.Passkeys != 1 || rep.TOTP != 1 || rep.Passwords != 2 {
		t.Fatalf("dry-run counts: %+v", rep)
	}
	for _, tbl := range []string{"theauth_users", "theauth_api_tokens", "theauth_webauthn_credentials", "theauth_totp_secrets", "authengine_user_map", "authengine_token_map"} {
		if n := f.count(t, tbl); n != 0 {
			t.Fatalf("%s has %d rows after dry run", tbl, n)
		}
	}
}

func TestBackfillIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := authengine.Backfill(ctx, f.db.DB, f.opts(false)); err != nil {
		t.Fatalf("first run: %v", err)
	}
	rep, err := authengine.Backfill(ctx, f.db.DB, f.opts(false))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if rep.Users != 0 || rep.Tokens != 0 || rep.Passkeys != 0 || rep.TOTP != 0 || rep.Passwords != 0 || rep.UsersAlreadyMapped != 2 {
		t.Fatalf("second run copied again: %+v", rep)
	}
	if f.count(t, "theauth_users") != 2 || f.count(t, "theauth_api_tokens") != 3 {
		t.Fatal("row counts changed on rerun")
	}
}

func TestBackfillRejectsCaseInsensitiveEmailCollision(t *testing.T) {
	f := newFixture(t)
	h := "x"
	err := f.db.CreateUser(context.Background(), store.User{
		ID: "user_cccc", Email: strings.ToUpper(legacyEmail), DisplayName: "Dup", PasswordHash: &h,
		Abilities: []string{"read"}, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authengine.Backfill(context.Background(), f.db.DB, f.opts(false)); err == nil {
		t.Fatal("expected a collision error")
	}
	if f.count(t, "theauth_users") != 0 {
		t.Fatal("collision must not leave partial rows")
	}
}

func TestBackfillRollsBackOnFailure(t *testing.T) {
	f := newFixture(t)
	f.addToken(t, "tok_bad", "user_aaaa", []string{"read"})
	if _, err := f.db.Exec(`UPDATE api_tokens SET abilities = '["bogus"]' WHERE id = 'tok_bad'`); err != nil {
		t.Fatal(err)
	}
	if _, err := authengine.Backfill(context.Background(), f.db.DB, f.opts(false)); err == nil {
		t.Fatal("expected an unknown ability error")
	}
	if f.count(t, "theauth_users") != 0 || f.count(t, "authengine_user_map") != 0 {
		t.Fatal("failed run left rows behind")
	}
}

func post(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b)) //nolint:gosec // test server URL
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestLegacyUserSignsInThroughLibraryAndRehashes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := authengine.Backfill(ctx, f.db.DB, f.opts(false)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	f.engine(t, srv)
	url := srv.URL + authengine.DefaultPathPrefix + "/email-password/signin"

	if resp := post(t, url, map[string]string{"email": legacyEmail, "password": "wrong password!!"}); resp.StatusCode == http.StatusOK {
		t.Fatal("wrong password must not sign in")
	}
	resp := post(t, url, map[string]string{"email": strings.ToLower(legacyEmail), "password": legacyPassword})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signin status = %d", resp.StatusCode)
	}
	var stored string
	if err := f.db.QueryRow(`SELECT p.password_hash FROM theauth_user_passwords p
		JOIN authengine_user_map m ON m.engine_id = p.user_id WHERE m.legacy_id = 'user_aaaa'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "$argon2id$") {
		t.Fatalf("hash not upgraded after login: prefix %.10s", stored)
	}
}

func TestLegacyTokensAuthenticateThroughLibrary(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := authengine.Backfill(ctx, f.db.DB, f.opts(false)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	eng := f.engine(t, srv)

	cases := []struct {
		id   string
		want []string
	}{
		{"tok_root", []string{theauth.AbilityRoot}},
		{"tok_read", []string{"read", "read:sensitive"}},
		{"tok_deploy", []string{"deploy"}},
	}
	for _, c := range cases {
		p, err := eng.Auth().AuthenticateAPIToken(ctx, f.rawTokens[c.id])
		if err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		for _, a := range c.want {
			if !p.Has(a) {
				t.Errorf("%s: missing ability %q, got %v", c.id, a, p.Abilities)
			}
		}
	}
	p, err := eng.Auth().AuthenticateAPIToken(ctx, f.rawTokens["tok_deploy"])
	if err != nil {
		t.Fatal(err)
	}
	if p.Has("write") || p.Has("read") {
		t.Fatalf("deploy token gained abilities: %v", p.Abilities)
	}
	if _, err := eng.Auth().AuthenticateAPIToken(ctx, f.rawTokens["tok_system"]); err == nil {
		t.Fatal("ownerless system token must not authenticate through the library")
	}
}

func TestAbilityMappingTable(t *testing.T) {
	cases := map[string]string{
		"read": "read", "read:sensitive": "read:sensitive", "write": "write",
		"write:sensitive": "write:sensitive", "deploy": "deploy", "root": theauth.AbilityRoot,
	}
	for legacy, want := range cases {
		got, err := authengine.MapAbilities([]string{legacy})
		if err != nil || len(got) != 1 || got[0] != want {
			t.Errorf("MapAbilities(%q) = %v, %v; want %q", legacy, got, err, want)
		}
	}
	if _, err := authengine.MapAbilities([]string{"bogus"}); err == nil {
		t.Error("unknown ability must be rejected")
	}
}

func TestTOTPAndPasskeyLandReadableByLibrary(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := authengine.Backfill(ctx, f.db.DB, f.opts(false)); err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.New(f.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	var adminEngine, devEngine string
	for legacy, dst := range map[string]*string{"user_aaaa": &adminEngine, "user_bbbb": &devEngine} {
		if err := f.db.QueryRow(`SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacy).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	adminID, err := ulid.Parse(adminEngine)
	if err != nil {
		t.Fatal(err)
	}
	sec, err := st.TOTPSecretByUserID(ctx, adminID)
	if err != nil || sec.ConfirmedAt == nil {
		t.Fatalf("TOTP secret: %v %+v", err, sec)
	}
	plain, err := crypto.Decrypt(f.key, sec.SecretEnc)
	if err != nil || string(plain) != totpBase32 {
		t.Fatalf("TOTP secret did not round-trip (err=%v)", err)
	}
	devID, err := ulid.Parse(devEngine)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := st.WebAuthnCredentialsByUserID(ctx, devID)
	if err != nil || len(creds) != 1 {
		t.Fatalf("passkeys: %v %d", err, len(creds))
	}
	c := creds[0]
	if string(c.CredentialID) != "cred-1" || c.SignCount != 7 || len(c.Transports) != 2 || c.BackupEligible == nil || !*c.BackupEligible {
		t.Fatalf("passkey not preserved: %+v", c)
	}
}
