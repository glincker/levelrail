package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRecoverAdminLibraryEndsSessionsWithoutRestart(t *testing.T) {
	t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
	t.Setenv(authengine.EnvAreas, string(authengine.AreaSessions))
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password-1"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := string(hash)
	if err := db.CreateUser(ctx, store.User{ID: "user_a", Email: "admin@example.test", DisplayName: "admin", PasswordHash: &h, Abilities: []string{"root"}, IsFirstUser: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(db.DB, authengine.Config{BaseURL: "http://example.test", Directory: authengine.NewDirectory(db.DB), Sessions: authengine.SessionsHooks{}})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	sess := eng.Sessions()
	tok, _, err := sess.Login(ctx, authengine.LoginInput{Email: "admin@example.test", Password: "old-password-1", IP: "198.51.100.9"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, path) }
	if err := runRecoverAdmin(ctx, logger, []string{"--username", "admin@example.test", "--password", "new-password-9"}, &out, open); err != nil {
		t.Fatalf("runRecoverAdmin: %v", err)
	}
	if !strings.Contains(out.String(), "sessions and login lockouts") || strings.Contains(out.String(), "restart the container") {
		t.Errorf("output = %q, want the library recovery note and no restart hint", out.String())
	}
	if _, ok := sess.Lookup(ctx, tok); ok {
		t.Error("old session survived recover-admin in library mode")
	}
	if _, _, err := sess.Login(ctx, authengine.LoginInput{Email: "admin@example.test", Password: "new-password-9", IP: "198.51.100.9"}); err != nil {
		t.Errorf("login with recovered password: %v", err)
	}
	if _, _, err := sess.Login(ctx, authengine.LoginInput{Email: "admin@example.test", Password: "old-password-1", IP: "198.51.100.9"}); err == nil {
		t.Error("old password still works")
	}
}
