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
)

func TestSessionsLoginWithEnrolledTOTPAsksForSecondFactor(t *testing.T) {
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
	u := store.User{ID: "user_mfa", Email: "mfa@example.test", DisplayName: "mfa", PasswordHash: &h, Abilities: []string{"read"}, CreatedAt: time.Now()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := eng.Sessions().SyncUser(ctx, authengine.LegacyUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, PasswordHash: h, CreatedAt: u.CreatedAt}); err != nil {
		t.Fatalf("SyncUser: %v", err)
	}
	m, err := authengine.NewMFA(eng, db.DB)
	if err != nil {
		t.Fatalf("NewMFA: %v", err)
	}
	enr, err := m.TOTPBegin(ctx, u.ID, u.Email)
	if err != nil {
		t.Fatalf("TOTPBegin: %v", err)
	}
	if _, err := m.TOTPFinish(ctx, u.ID, codeAt(t, enr.Secret, time.Now())); err != nil {
		t.Fatalf("TOTPFinish: %v", err)
	}

	tok, id, err := eng.Sessions().Login(ctx, authengine.LoginInput{Email: u.Email, Password: sessPassword, UserAgent: "test", IP: "198.51.100.9"})
	if !errors.Is(err, authengine.ErrSecondFactorRequired) || id != u.ID || tok != "" {
		t.Fatalf("Login = (%q, %q, %v), want no token, %s, ErrSecondFactorRequired", tok, id, err, u.ID)
	}
	var live int
	if err := db.QueryRow(`SELECT COUNT(*) FROM theauth_sessions WHERE revoked_at IS NULL`).Scan(&live); err != nil || live != 0 {
		t.Fatalf("live library sessions after a pending login = %d (err %v), want 0", live, err)
	}
}
