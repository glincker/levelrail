package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func openRecoverAdminTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestParseRecoverAdminArgs_Defaults(t *testing.T) {
	username, password, err := parseRecoverAdminArgs(nil)
	if err != nil {
		t.Fatalf("parseRecoverAdminArgs() error = %v", err)
	}
	if username != "admin" {
		t.Errorf("username = %q, want %q (default)", username, "admin")
	}
	if password != "" {
		t.Errorf("password = %q, want empty (triggers generation)", password)
	}
}

func TestParseRecoverAdminArgs_ExplicitFlags(t *testing.T) {
	username, password, err := parseRecoverAdminArgs([]string{"--username", "root", "--password", "hunter2hunter2"})
	if err != nil {
		t.Fatalf("parseRecoverAdminArgs() error = %v", err)
	}
	if username != "root" {
		t.Errorf("username = %q, want %q", username, "root")
	}
	if password != "hunter2hunter2" {
		t.Errorf("password = %q, want %q", password, "hunter2hunter2")
	}
}

func TestParseRecoverAdminArgs_RejectsUnknownFlag(t *testing.T) {
	_, _, err := parseRecoverAdminArgs([]string{"--bogus", "x"})
	if err == nil {
		t.Fatal("parseRecoverAdminArgs() error = nil, want error for unknown flag")
	}
}

func TestGenerateRandomPassword_LengthAndUniqueness(t *testing.T) {
	a, err := generateRandomPassword()
	if err != nil {
		t.Fatalf("generateRandomPassword() error = %v", err)
	}
	if len(a) < 20 {
		t.Errorf("len(password) = %d, want >= 20", len(a))
	}
	b, err := generateRandomPassword()
	if err != nil {
		t.Fatalf("generateRandomPassword() error = %v", err)
	}
	if a == b {
		t.Errorf("two generated passwords were identical: %q", a)
	}
}

func TestRecoverAdminUser_CreatesUserWhenNoneExists(t *testing.T) {
	db := openRecoverAdminTestDB(t)
	ctx := context.Background()

	if err := recoverAdminUser(ctx, db, "admin", "correct horse battery staple"); err != nil {
		t.Fatalf("recoverAdminUser() error = %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.PasswordHash == nil {
		t.Fatal("PasswordHash = nil, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*got.PasswordHash), []byte("correct horse battery staple")); err != nil {
		t.Errorf("stored hash does not verify against the password just set: %v", err)
	}
	if got.IsFirstUser {
		t.Error("IsFirstUser = true, want false: recovery must not claim first-user status")
	}
	if len(got.Abilities) != 1 || got.Abilities[0] != api.AbilityRoot {
		t.Errorf("Abilities = %v, want [%q]: a recovered account with no abilities could log in but do nothing", got.Abilities, api.AbilityRoot)
	}
}

func TestRecoverAdminUser_ResetsExistingUsersPassword(t *testing.T) {
	db := openRecoverAdminTestDB(t)
	ctx := context.Background()

	seeded := store.User{ID: "user_1", Email: "admin", DisplayName: "admin", CreatedAt: time.Now()}
	if err := db.CreateUser(ctx, seeded); err != nil {
		t.Fatalf("seed CreateUser() error = %v", err)
	}

	if err := recoverAdminUser(ctx, db, "admin", "new-password-123"); err != nil {
		t.Fatalf("recoverAdminUser() error = %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.ID != seeded.ID {
		t.Errorf("ID = %q, want unchanged %q (must reset in place, not create a duplicate)", got.ID, seeded.ID)
	}
	if got.PasswordHash == nil {
		t.Fatal("PasswordHash = nil, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*got.PasswordHash), []byte("new-password-123")); err != nil {
		t.Errorf("stored hash does not verify against the new password: %v", err)
	}
}

func TestRunRecoverAdmin_GeneratesAndPrintsPasswordWhenOmitted(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	var stdout bytes.Buffer

	// runRecoverAdmin closes whatever *store.DB openStore hands it, the
	// same lifecycle main.go's run() uses for its own openStore call, so
	// each invocation here opens (and this test later reopens) the same
	// on-disk file rather than sharing one live handle across both.
	err := runRecoverAdmin(ctx, logger, []string{"--username", "admin"}, &stdout, func(ctx context.Context) (*store.DB, error) {
		return store.Open(ctx, dbPath)
	})
	if err != nil {
		t.Fatalf("runRecoverAdmin() error = %v", err)
	}

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "admin") {
		t.Errorf("stdout = %q, want it to mention the username", output)
	}

	// The generated password must appear in stdout and must actually
	// verify against the hash written to the store: that round trip is
	// the entire point of a recovery command someone runs after being
	// locked out.
	marker := "password: "
	idx := strings.Index(output, marker)
	if idx == -1 {
		t.Fatalf("stdout = %q, want it to contain %q followed by the generated password", output, marker)
	}
	rest := output[idx+len(marker):]
	if nl := strings.IndexByte(rest, '\n'); nl != -1 {
		rest = rest[:nl]
	}
	printed := strings.TrimSpace(rest)
	if got.PasswordHash == nil {
		t.Fatal("PasswordHash = nil, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*got.PasswordHash), []byte(printed)); err != nil {
		t.Errorf("printed password does not verify against the stored hash: %v", err)
	}
}

func TestRunRecoverAdmin_UsesExplicitPasswordWithoutPrintingIt(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	var stdout bytes.Buffer

	err := runRecoverAdmin(ctx, logger, []string{"--username", "admin", "--password", "explicit-pass-1"}, &stdout, func(ctx context.Context) (*store.DB, error) {
		return store.Open(ctx, dbPath)
	})
	if err != nil {
		t.Fatalf("runRecoverAdmin() error = %v", err)
	}

	if strings.Contains(stdout.String(), "explicit-pass-1") {
		t.Errorf("stdout = %q, must not echo back a caller-supplied password", stdout.String())
	}

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.PasswordHash == nil {
		t.Fatal("PasswordHash = nil, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*got.PasswordHash), []byte("explicit-pass-1")); err != nil {
		t.Errorf("stored hash does not verify against the explicit password: %v", err)
	}
}

func TestRunRecoverAdmin_OpenStoreFailurePropagates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	var stdout bytes.Buffer
	wantErr := errors.New("boom")

	err := runRecoverAdmin(context.Background(), logger, nil, &stdout, func(context.Context) (*store.DB, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runRecoverAdmin() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestRunRecoverAdmin_RetiresSignInGrants(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }
	args := []string{"--username", "admin", "--password", "explicit-pass-1"}
	if err := runRecoverAdmin(ctx, logger, args, &bytes.Buffer{}, open); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	user, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(db.DB, authengine.Config{BaseURL: "http://localhost", Directory: authengine.NewDirectory(db.DB)})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	expires := time.Now().Add(time.Hour)
	_, approver, err := eng.MintToken(ctx, authengine.MintInput{OwnerLegacyID: user.ID, Name: "approver", Abilities: []string{api.AbilitySignInApprove}, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := eng.MintToken(ctx, authengine.MintInput{OwnerLegacyID: user.ID, Name: "reader", Abilities: []string{api.AbilityRead}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, a := range []store.LoginApproval{
		{ID: "la_pending", UserID: user.ID, BrowserHash: "b1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "la_approved", UserID: user.ID, BrowserHash: "b2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := db.CreateLoginApproval(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DecideLoginApproval(ctx, "la_approved", user.ID, true, "session:x", now); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateLoginCodeChallenge(ctx, store.LoginCodeChallenge{ID: "lc_1", UserID: user.ID, BrowserHash: "b3", CodeHash: "h", Salt: "s",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	if err := runRecoverAdmin(ctx, logger, args, &bytes.Buffer{}, open); err != nil {
		t.Fatal(err)
	}

	if ok, err := db.ConsumeLoginApproval(ctx, "la_approved", time.Now()); err != nil || ok {
		t.Fatalf("approved approval still consumable after recovery: %v %v", ok, err)
	}
	if pending, _ := db.ListPendingLoginApprovalsForUser(ctx, user.ID, time.Now()); len(pending) != 0 {
		t.Fatalf("pending approvals = %d after recovery", len(pending))
	}
	if live, _ := db.ListLiveLoginCodesForUser(ctx, user.ID, time.Now()); len(live) != 0 {
		t.Fatalf("live codes = %d after recovery", len(live))
	}
	tests := []struct {
		name    string
		id      string
		revoked bool
	}{
		{"signin:approve token is revoked", approver.EngineID, true},
		{"other tokens are kept", reader.EngineID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := eng.GetToken(ctx, tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if got := rec.RevokedAt != nil; got != tt.revoked {
				t.Fatalf("revoked = %v, want %v", got, tt.revoked)
			}
		})
	}
}
