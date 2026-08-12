package store

import (
	"context"
	"errors"
	"testing"
)

func TestGetAdminUser_NotFoundBeforeAnyUpsert(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetAdminUser(context.Background())
	if !errors.Is(err, ErrAdminNotFound) {
		t.Errorf("error = %v, want ErrAdminNotFound", err)
	}
}

func TestUpsertAndGetAdminUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpsertAdminUser(ctx, "admin", "hashed-1"); err != nil {
		t.Fatalf("UpsertAdminUser() error = %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin" || got.PasswordHash != "hashed-1" {
		t.Errorf("got %+v, want Username=admin PasswordHash=hashed-1", got)
	}
}

func TestUpsertAdminUser_ReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpsertAdminUser(ctx, "admin", "hashed-1"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := db.UpsertAdminUser(ctx, "admin2", "hashed-2"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin2" || got.PasswordHash != "hashed-2" {
		t.Errorf("got %+v, want the second upsert's values (single-row table)", got)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_user`).Scan(&count); err != nil {
		t.Fatalf("count admin_user rows: %v", err)
	}
	if count != 1 {
		t.Errorf("admin_user row count = %d, want exactly 1 (CHECK id = 1 constraint)", count)
	}
}
