package api

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMaybeBootstrapDevAdmin_Disabled_NoOp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := maybeBootstrapDevAdmin(ctx, db, logger, false); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	if n, err := db.CountUsers(ctx); err != nil || n != 0 {
		t.Errorf("CountUsers() = (%d, %v), want (0, nil) (disabled must not create an account)", n, err)
	}
}

func TestMaybeBootstrapDevAdmin_Enabled_CreatesDevAdmin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := maybeBootstrapDevAdmin(ctx, db, logger, true); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	got, err := db.GetUserByEmail(ctx, devModeUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.Email != devModeUsername {
		t.Errorf("email = %q, want %q", got.Email, devModeUsername)
	}
	if got.PasswordHash == nil {
		t.Fatal("PasswordHash = nil, want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*got.PasswordHash), []byte(devModePassword)); err != nil {
		t.Errorf("stored hash does not verify against the fixed dev password: %v", err)
	}
}

func TestMaybeBootstrapDevAdmin_Enabled_DoesNotClobberExistingAdmin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	seeded := storeUserForTest(t, db, "admin")
	realHash := "real-hash"
	if err := db.UpdateUserPasswordHash(ctx, seeded.ID, &realHash); err != nil {
		t.Fatalf("seed UpdateUserPasswordHash() error = %v", err)
	}

	if err := maybeBootstrapDevAdmin(ctx, db, logger, true); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if got.Email != "admin" || got.PasswordHash == nil || *got.PasswordHash != "real-hash" {
		t.Errorf("got %+v, want the pre-existing admin left untouched (BootstrapAdmin is a no-op once any user exists)", got)
	}
	if n, err := db.CountUsers(ctx); err != nil || n != 1 {
		t.Errorf("CountUsers() = (%d, %v), want (1, nil): dev bootstrap must not have added a second user", n, err)
	}
}

func TestMaybeBootstrapDevAdmin_Enabled_LogsWarning(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	if err := maybeBootstrapDevAdmin(ctx, db, logger, true); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte(devModeUsername)) {
		t.Errorf("log output = %q, want it to mention the dev username so the bypass is unmissable in startup logs", buf.String())
	}
}
