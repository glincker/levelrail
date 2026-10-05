package authengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/totp"
)

func newMFAForTest(t *testing.T, f *fixture) *authengine.MFA {
	t.Helper()
	t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
	t.Setenv(authengine.EnvAreas, "")
	if _, err := authengine.Backfill(context.Background(), f.db.DB, f.opts(false)); err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	eng, err := authengine.New(f.db.DB, authengine.Config{
		BaseURL: "https://levelrail.test", TokenPrefix: "tk", EncryptionKey: f.key, TOTPIssuer: "test",
		Directory: authengine.NewDirectory(f.db.DB),
		MFA:       authengine.MFAConfig{DashboardURL: "https://levelrail.test"},
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	m, err := authengine.NewMFA(eng, f.db.DB)
	if err != nil {
		t.Fatalf("NewMFA: %v", err)
	}
	return m
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	return c
}

func TestMFABackfilledTOTPVerifiesAndReplayIsRejected(t *testing.T) {
	ctx := context.Background()
	m := newMFAForTest(t, newFixture(t))
	now := time.Now()

	st, err := m.TOTPStatus(ctx, "user_aaaa")
	if err != nil {
		t.Fatalf("TOTPStatus: %v", err)
	}
	if !st.Enabled || st.RecoveryCodesRemaining != 0 || !st.NeedsRegeneration {
		t.Fatalf("backfilled status = %+v, want enabled with no recovery codes and a regeneration flag", st)
	}

	code := codeAt(t, totpBase32, now)
	if err := m.CheckSecondFactor(ctx, "user_aaaa", code, "", "ua", "203.0.113.5"); err != nil {
		t.Fatalf("backfilled secret must verify a real code: %v", err)
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
			m := newMFAForTest(t, newFixture(t))
			for i := 0; i < tc.bad; i++ {
				if err := m.CheckSecondFactor(ctx, "user_aaaa", "000000", "", "ua", "203.0.113.5"); !errors.Is(err, authengine.ErrInvalidCode) {
					t.Fatalf("bad attempt %d: err = %v, want ErrInvalidCode", i, err)
				}
			}
			err := m.CheckSecondFactor(ctx, "user_aaaa", codeAt(t, totpBase32, time.Now()), "", "ua", "203.0.113.5")
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
	m := newMFAForTest(t, newFixture(t))
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
	if err != nil || !st.Enabled || st.RecoveryCodesRemaining != 9 || st.NeedsRegeneration {
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
	m := newMFAForTest(t, newFixture(t))
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
