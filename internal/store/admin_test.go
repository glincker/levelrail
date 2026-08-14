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

func TestCreateAdminUser_Success(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.CreateAdminUser(ctx, "admin", "hashed-1"); err != nil {
		t.Fatalf("CreateAdminUser() error = %v", err)
	}

	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin" || got.PasswordHash != "hashed-1" {
		t.Errorf("got %+v, want Username=admin PasswordHash=hashed-1", got)
	}
}

func TestCreateAdminUser_AlreadyExists_DoesNotOverwrite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.CreateAdminUser(ctx, "admin", "hashed-1"); err != nil {
		t.Fatalf("first CreateAdminUser() error = %v", err)
	}

	err := db.CreateAdminUser(ctx, "attacker", "hashed-attacker")
	if !errors.Is(err, ErrAdminAlreadyExists) {
		t.Fatalf("second CreateAdminUser() error = %v, want ErrAdminAlreadyExists", err)
	}

	// The critical property: unlike UpsertAdminUser, a second
	// CreateAdminUser call must never replace the existing account.
	got, err := db.GetAdminUser(ctx)
	if err != nil {
		t.Fatalf("GetAdminUser() error = %v", err)
	}
	if got.Username != "admin" || got.PasswordHash != "hashed-1" {
		t.Errorf("got %+v, want the original account unchanged (Username=admin PasswordHash=hashed-1)", got)
	}
}

func TestCreateAdminUser_ConcurrentCallers_OnlyOneSucceeds(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	const n = 20
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			results <- db.CreateAdminUser(ctx, "admin", "hashed")
		}()
	}

	succeeded := 0
	for i := 0; i < n; i++ {
		if err := <-results; err == nil {
			succeeded++
		} else if !errors.Is(err, ErrAdminAlreadyExists) {
			t.Errorf("unexpected error from concurrent CreateAdminUser: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("succeeded = %d out of %d concurrent CreateAdminUser calls, want exactly 1: the database's own PRIMARY KEY constraint must be what decides, not a race-prone caller-side check", succeeded, n)
	}
}
