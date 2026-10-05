package authengine_test

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/totp"
)

type mfaEnv struct {
	*authengine.MFA
	secretA string
}

func newMFAForTest(t *testing.T) *mfaEnv {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL: "https://levelrail.test", TokenPrefix: "tk", EncryptionKey: key, TOTPIssuer: "test",
		Directory: authengine.NewDirectory(db.DB),
		MFA:       authengine.MFAConfig{DashboardURL: "https://levelrail.test"},
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	hash, err := bcrypt.GenerateFromPassword([]byte(sessPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := string(hash)
	for _, u := range []store.User{
		{ID: "user_aaaa", Email: "admin@example.test", DisplayName: "Admin", PasswordHash: &h, Abilities: []string{"root"}, IsFirstUser: true, CreatedAt: time.Now()},
		{ID: "user_bbbb", Email: "dev@example.test", DisplayName: "Dev", PasswordHash: &h, Abilities: []string{"read"}, CreatedAt: time.Now()},
	} {
		if err := db.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if _, err := eng.Sessions().SyncUser(ctx, authengine.LegacyUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, PasswordHash: h, CreatedAt: u.CreatedAt}); err != nil {
			t.Fatalf("SyncUser: %v", err)
		}
	}
	m, err := authengine.NewMFA(eng, db.DB)
	if err != nil {
		t.Fatalf("NewMFA: %v", err)
	}
	enr, err := m.TOTPBegin(ctx, "user_aaaa", "admin@example.test")
	if err != nil {
		t.Fatalf("TOTPBegin: %v", err)
	}
	if _, err := m.TOTPFinish(ctx, "user_aaaa", codeAt(t, enr.Secret, time.Now())); err != nil {
		t.Fatalf("TOTPFinish: %v", err)
	}
	return &mfaEnv{MFA: m, secretA: enr.Secret}
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	return c
}

func TestMFAEnrolledTOTPVerifiesAndReplayIsRejected(t *testing.T) {
	ctx := context.Background()
	env := newMFAForTest(t)
	m := env.MFA
	code := codeAt(t, env.secretA, time.Now())
	if err := m.CheckSecondFactor(ctx, "user_aaaa", code, "", "ua", "203.0.113.5"); err != nil {
		t.Fatalf("enrolled secret must verify a real code: %v", err)
	}
	if err := m.CheckSecondFactor(ctx, "user_aaaa", code, "", "ua", "203.0.113.5"); !errors.Is(err, authengine.ErrInvalidCode) {
		t.Fatalf("replayed code: err = %v, want ErrInvalidCode", err)
	}
}

func TestMFALockoutAfterRepeatedBadCodes(t *testing.T) {
	tests := []struct {
		name       string
		bad        int
		wantLocked bool
	}{
		{"below the limit still verifies", 4, false},
		{"at the limit locks even a good code", 5, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			env := newMFAForTest(t)
			m := env.MFA
			for i := 0; i < tc.bad; i++ {
				if err := m.CheckSecondFactor(ctx, "user_aaaa", "000000", "", "ua", "203.0.113.5"); !errors.Is(err, authengine.ErrInvalidCode) {
					t.Fatalf("bad attempt %d: err = %v, want ErrInvalidCode", i, err)
				}
			}
			err := m.CheckSecondFactor(ctx, "user_aaaa", codeAt(t, env.secretA, time.Now()), "", "ua", "203.0.113.5")
			var locked *authengine.LockedError
			if tc.wantLocked {
				if !errors.As(err, &locked) || locked.RetryAfter <= 0 {
					t.Fatalf("err = %v, want LockedError with a retry delay", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("good code after %d bad ones: %v", tc.bad, err)
			}
		})
	}
}

func TestMFAEnrollRecoveryRegenerateDisable(t *testing.T) {
	ctx := context.Background()
	m := newMFAForTest(t).MFA
	now := time.Now()

	enroll, err := m.TOTPBegin(ctx, "user_bbbb", "dev@example.test")
	if err != nil {
		t.Fatalf("TOTPBegin: %v", err)
	}
	if _, err := m.TOTPFinish(ctx, "user_bbbb", "000000"); !errors.Is(err, authengine.ErrInvalidCode) {
		t.Fatalf("bad confirm code: err = %v, want ErrInvalidCode", err)
	}
	codes, err := m.TOTPFinish(ctx, "user_bbbb", codeAt(t, enroll.Secret, now))
	if err != nil || len(codes) != 10 {
		t.Fatalf("TOTPFinish: codes=%d err=%v", len(codes), err)
	}
	if _, err := m.TOTPFinish(ctx, "user_bbbb", codeAt(t, enroll.Secret, now)); !errors.Is(err, authengine.ErrNoEnrollment) {
		t.Fatalf("second finish: err = %v, want ErrNoEnrollment", err)
	}

	if err := m.CheckSecondFactor(ctx, "user_bbbb", "", codes[0], "ua", "203.0.113.6"); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	if err := m.CheckSecondFactor(ctx, "user_bbbb", "", codes[0], "ua", "203.0.113.6"); !errors.Is(err, authengine.ErrInvalidCode) {
		t.Fatalf("reused recovery code: err = %v, want ErrInvalidCode", err)
	}
	st, err := m.TOTPStatus(ctx, "user_bbbb")
	if err != nil || !st.Enabled || st.RecoveryCodesRemaining != 9 {
		t.Fatalf("status after one recovery use = %+v, %v", st, err)
	}
	fresh, err := m.RegenerateRecoveryCodes(ctx, "user_bbbb")
	if err != nil || len(fresh) != 10 {
		t.Fatalf("regenerate: %d codes, %v", len(fresh), err)
	}
	if err := m.CheckSecondFactor(ctx, "user_bbbb", "", codes[1], "ua", "203.0.113.6"); !errors.Is(err, authengine.ErrInvalidCode) {
		t.Fatalf("pre-regeneration code must stop working: %v", err)
	}
	if err := m.TOTPDisable(ctx, "user_bbbb"); err != nil {
		t.Fatalf("TOTPDisable: %v", err)
	}
	if st, _ := m.TOTPStatus(ctx, "user_bbbb"); st.Enabled {
		t.Fatalf("still enabled after disable: %+v", st)
	}
}

func TestMFAUnmappedUser(t *testing.T) {
	m := newMFAForTest(t).MFA
	if _, err := m.TOTPStatus(context.Background(), "user_ghost"); !errors.Is(err, authengine.ErrNotMapped) {
		t.Fatalf("err = %v, want ErrNotMapped", err)
	}
}

func TestMFAConfigFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		dashboard  string
		wantRPID   string
		wantOrigin []string
		wantUV     bool
		wantClone  string
	}{
		{name: "derived from the dashboard url", dashboard: "https://deploy.example.com", wantRPID: "deploy.example.com", wantOrigin: []string{"https://deploy.example.com"}, wantClone: "reject"},
		{name: "port stays in the origin only", dashboard: "https://deploy.example.com:8443", wantRPID: "deploy.example.com", wantOrigin: []string{"https://deploy.example.com:8443"}, wantClone: "reject"},
		{name: "ip hosts yield no relying party", dashboard: "http://203.0.113.9:9000", wantClone: "reject"},
		{name: "env overrides and extra origins", dashboard: "https://deploy.example.com", env: map[string]string{
			authengine.EnvWebAuthnRPID: "example.com", authengine.EnvWebAuthnOrigins: "https://a.example.com/, https://deploy.example.com",
			authengine.EnvWebAuthnRequireUV: "true", authengine.EnvWebAuthnCloneWarning: "flag",
		}, wantRPID: "example.com", wantOrigin: []string{"https://deploy.example.com", "https://a.example.com"}, wantUV: true, wantClone: "flag"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got := authengine.MFAConfigFromEnv(authengine.MFAConfig{DashboardURL: tc.dashboard})
			if got.RPID != tc.wantRPID || got.RequireUserVerification != tc.wantUV || string(got.CloneWarning) != tc.wantClone {
				t.Fatalf("got %+v", got)
			}
			if len(got.RPOrigins) != len(tc.wantOrigin) {
				t.Fatalf("origins = %v, want %v", got.RPOrigins, tc.wantOrigin)
			}
			for i := range tc.wantOrigin {
				if got.RPOrigins[i] != tc.wantOrigin[i] {
					t.Fatalf("origins = %v, want %v", got.RPOrigins, tc.wantOrigin)
				}
			}
			if got.MaxFailures != 5 || got.Lockout != 15*time.Minute {
				t.Fatalf("defaults = %d / %s", got.MaxFailures, got.Lockout)
			}
		})
	}
}
