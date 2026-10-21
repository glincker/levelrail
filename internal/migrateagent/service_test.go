package migrateagent

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/agent/pinned"
	"github.com/GLINCKER/levelrail/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fixture struct {
	svc *Service
	db  *store.DB
	clk *clock
	ca  *agent.CA
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ca, err := agent.GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Now().UTC().Truncate(time.Second)}
	svc := NewService(db, ca, Config{TokenTTL: time.Hour, SessionMax: 72 * time.Hour, RequestTimeout: time.Second}, nil, clk.now)
	return fixture{svc: svc, db: db, clk: clk, ca: ca}
}

func (f fixture) enroll(t *testing.T, token string) (*migrationpb.EnrollResponse, error) {
	t.Helper()
	_, csr, err := pinned.NewKeyAndCSR("migration-source")
	if err != nil {
		t.Fatal(err)
	}
	return f.svc.Enroll(context.Background(), &migrationpb.EnrollRequest{JoinToken: token, CsrDer: csr, Version: "v1", Os: "linux", Arch: "amd64", CaPinState: "pinned"})
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("got %v (code %v), want code %v", err, status.Code(err), want)
	}
}

func auditActions(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

func hasAction(actions []string, a string) bool {
	for _, x := range actions {
		if x == a {
			return true
		}
	}
	return false
}

func TestEnrollTokenIsSingleUse(t *testing.T) {
	f := newFixture(t)
	c, err := f.svc.Create(context.Background(), "old server", "alice")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := f.enroll(t, c.Token)
	if err != nil {
		t.Fatalf("first enroll: %v", err)
	}
	block, _ := pem.Decode(resp.GetClientCertPem())
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.Subject.CommonName != CertCommonNamePrefix+c.ID {
		t.Fatalf("bad certificate: %v %v", cert, err)
	}
	if resp.GetSessionExpiresAt().AsTime().Sub(f.clk.now()) != 72*time.Hour {
		t.Fatalf("session cap not applied: %v", resp.GetSessionExpiresAt().AsTime())
	}
	_, err = f.enroll(t, c.Token)
	wantCode(t, err, codes.Unauthenticated)
	actions := auditActions(t, f.db)
	for _, want := range []string{ActionTokenIssued, ActionEnrolled, ActionEnrollRefused} {
		if !hasAction(actions, want) {
			t.Errorf("audit log is missing %s: %v", want, actions)
		}
	}
}

func TestEnrollRefusals(t *testing.T) {
	cases := []struct {
		name  string
		token func(t *testing.T, f fixture) string
		clock time.Duration
	}{
		{"expired", func(t *testing.T, f fixture) string { return mustCreate(t, f).Token }, 61 * time.Minute},
		{"wrong secret", func(t *testing.T, f fixture) string {
			c := mustCreate(t, f)
			return tokenPrefix + c.ID + "." + strings.Repeat("A", 43)
		}, 0},
		{"unknown session", func(*testing.T, fixture) string {
			return tokenPrefix + "ms_0000000000000000." + strings.Repeat("B", 43)
		}, 0},
		{"malformed", func(*testing.T, fixture) string { return "not-a-token" }, 0},
		{"empty", func(*testing.T, fixture) string { return "" }, 0},
		{"node join token shape", func(*testing.T, fixture) string { return strings.Repeat("a", 64) }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tok := tc.token(t, f)
			f.clk.advance(tc.clock)
			_, err := f.enroll(t, tok)
			wantCode(t, err, codes.Unauthenticated)
		})
	}
}

func mustCreate(t *testing.T, f fixture) Created {
	t.Helper()
	c, err := f.svc.Create(context.Background(), "x", "alice")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f fixture) enrolledCert(t *testing.T) (Created, *x509.Certificate) {
	t.Helper()
	c := mustCreate(t, f)
	resp, err := f.enroll(t, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(resp.GetClientCertPem())
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c, cert
}

func TestSessionCertificateChecks(t *testing.T) {
	f := newFixture(t)
	c, cert := f.enrolledCert(t)
	ctx := context.Background()

	if _, err := f.svc.sessionForCert(ctx, cert); err != nil {
		t.Fatalf("a fresh certificate must be accepted: %v", err)
	}

	nodeCert := *cert
	nodeCert.Subject.CommonName = "node-abc123"
	_, err := f.svc.sessionForCert(ctx, &nodeCert)
	wantCode(t, err, codes.PermissionDenied)

	other := *cert
	other.Raw = append([]byte(nil), cert.Raw...)
	other.Raw[len(other.Raw)-1] ^= 0xff
	_, err = f.svc.sessionForCert(ctx, &other)
	wantCode(t, err, codes.PermissionDenied)

	if err := f.svc.Revoke(ctx, c.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.sessionForCert(ctx, cert)
	wantCode(t, err, codes.PermissionDenied)
	if _, err := f.svc.Discover(ctx, c.ID); !errors.Is(err, ErrNotActive) {
		t.Fatalf("Discover on a revoked session got %v, want ErrNotActive", err)
	}
	actions := auditActions(t, f.db)
	if !hasAction(actions, ActionRevoked) || !hasAction(actions, ActionSessionRejected) {
		t.Fatalf("revoke and rejection must be audited: %v", actions)
	}
}

func TestSessionRejectedAfterCap(t *testing.T) {
	f := newFixture(t)
	_, cert := f.enrolledCert(t)
	f.clk.advance(73 * time.Hour)
	_, err := f.svc.sessionForCert(context.Background(), cert)
	wantCode(t, err, codes.PermissionDenied)
}

func TestRevokeSticksWhenAgentIsOffline(t *testing.T) {
	f := newFixture(t)
	c, _ := f.enrolledCert(t)
	if err := f.svc.Revoke(context.Background(), c.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	v, _, _, err := f.svc.Get(context.Background(), c.ID)
	if err != nil || v.State != store.MigrationAgentRevoked {
		t.Fatalf("state %q err %v, want revoked", v.State, err)
	}
	if _, err := f.svc.Scan(context.Background(), c.ID, []string{"x"}); !errors.Is(err, ErrNotActive) {
		t.Fatalf("Scan got %v, want ErrNotActive", err)
	}
}

func TestDiscoverOfflineAgent(t *testing.T) {
	f := newFixture(t)
	c, _ := f.enrolledCert(t)
	if _, err := f.svc.Discover(context.Background(), c.ID); !errors.Is(err, ErrOffline) {
		t.Fatalf("got %v, want ErrOffline", err)
	}
}

func TestEffectiveState(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	cases := []struct {
		name string
		sess store.MigrationAgentSession
		live bool
		want string
	}{
		{"pending valid", store.MigrationAgentSession{State: "pending", TokenExpiresAt: future}, false, "pending"},
		{"pending expired", store.MigrationAgentSession{State: "pending", TokenExpiresAt: past}, false, "expired"},
		{"connected live", store.MigrationAgentSession{State: "connected", SessionExpiresAt: &future}, true, "connected"},
		{"connected not live", store.MigrationAgentSession{State: "connected", SessionExpiresAt: &future}, false, "lost"},
		{"past cap", store.MigrationAgentSession{State: "connected", SessionExpiresAt: &past}, true, "expired"},
		{"revoked stays", store.MigrationAgentSession{State: "revoked", SessionExpiresAt: &future}, true, "revoked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveState(tc.sess, tc.live, now); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTokenParsing(t *testing.T) {
	id, tok, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	gotID, secret, ok := ParseToken(tok)
	if !ok || gotID != id || HashSecret(secret) != hash || !SecretMatches(hash, secret) || SecretMatches(hash, secret+"x") {
		t.Fatalf("token round trip failed: %v %v", gotID, ok)
	}
	for _, bad := range []string{"", "mig_", "mig_ms_1.", "mig_ms_abcdef12.short", "tok_ms_abcdef12." + strings.Repeat("a", 40), "mig_xx_abcdef12." + strings.Repeat("a", 40)} {
		if _, _, ok := ParseToken(bad); ok {
			t.Errorf("ParseToken(%q) should fail", bad)
		}
	}
	if strings.Contains(hash, tok) || strings.Contains(hash, secret) {
		t.Fatal("hash must not contain the token")
	}
}

func TestConfigClampsEnv(t *testing.T) {
	t.Setenv("APP_MIGRATION_TOKEN_TTL", "1ns")
	t.Setenv("APP_MIGRATION_SESSION_MAX", "9999h")
	c := ConfigFromEnv()
	if c.TokenTTL != minTokenTTL || c.SessionMax != maxSessionMax {
		t.Fatalf("not clamped: %+v", c)
	}
	t.Setenv("APP_MIGRATION_TOKEN_TTL", "")
	t.Setenv("APP_MIGRATION_SESSION_MAX", "")
	c = ConfigFromEnv()
	if c.TokenTTL != time.Hour || c.SessionMax != 72*time.Hour {
		t.Fatalf("defaults wrong: %+v", c)
	}
}
