package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMaybeBootstrapDevAdmin_Disabled_NoOp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := maybeBootstrapDevAdmin(ctx, db, logger, false); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	if _, err := db.GetAdminUser(ctx); !errors.Is(err, store.ErrAdminNotFound) {
		t.Errorf("GetAdminUser() error = %v, want ErrAdminNotFound (disabled must not create an account)", err)
	}
}

func TestMaybeBootstrapDevAdmin_Enabled_CreatesDevAdmin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := maybeBootstrapDevAdmin(ctx, db, logger, true); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != devModeUsername {
		t.Errorf("username = %q, want %q", got.Username, devModeUsername)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte(devModePassword)); err != nil {
		t.Errorf("stored hash does not verify against the fixed dev password: %v", err)
	}
}

func TestMaybeBootstrapDevAdmin_Enabled_DoesNotClobberExistingAdmin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))

	if err := db.UpsertAdminUser(ctx, "admin", "real-hash"); err != nil {
		t.Fatalf("seed UpsertAdminUser() error = %v", err)
	}

	if err := maybeBootstrapDevAdmin(ctx, db, logger, true); err != nil {
		t.Fatalf("maybeBootstrapDevAdmin() error = %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin" || got.PasswordHash != "real-hash" {
		t.Errorf("got %+v, want the pre-existing admin left untouched (BootstrapAdmin is a no-op once any admin exists)", got)
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
