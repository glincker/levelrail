package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndGetUser_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	hash := "bcrypt-hash"
	want := User{
		ID:           "user_abc123",
		Email:        "founder@example.com",
		DisplayName:  "Founder",
		PasswordHash: &hash,
		IsFirstUser:  true,
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := db.CreateUser(ctx, want); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	byID, err := db.GetUserByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if byID.Email != want.Email || byID.DisplayName != want.DisplayName || !byID.IsFirstUser {
		t.Errorf("GetUserByID() = %+v, want %+v", byID, want)
	}
	if byID.PasswordHash == nil || *byID.PasswordHash != hash {
		t.Errorf("PasswordHash = %v, want %q", byID.PasswordHash, hash)
	}

	byEmail, err := db.GetUserByEmail(ctx, want.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if byEmail.ID != want.ID {
		t.Errorf("GetUserByEmail().ID = %q, want %q", byEmail.ID, want.ID)
	}
}

func TestCreateUser_OAuthOnlyHasNilPasswordHash(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u := User{
		ID:          "user_oauth1",
		Email:       "oauth@example.com",
		DisplayName: "OAuth User",
		CreatedAt:   time.Now().UTC(),
	}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.PasswordHash != nil {
		t.Errorf("PasswordHash = %v, want nil for an OAuth-only user", got.PasswordHash)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetUserByID(ctx, "does-not-exist"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("GetUserByID() error = %v, want ErrUserNotFound", err)
	}
}

func TestCreateUser_DuplicateEmail_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := User{ID: "user_1", Email: "dup@example.com", DisplayName: "First", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, first); err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	second := User{ID: "user_2", Email: "dup@example.com", DisplayName: "Second", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, second); !errors.Is(err, ErrUserEmailExists) {
		t.Errorf("second CreateUser() error = %v, want ErrUserEmailExists", err)
	}
}

func TestCreateUser_SecondFirstUser_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := User{ID: "user_1", Email: "one@example.com", DisplayName: "One", IsFirstUser: true, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, first); err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	second := User{ID: "user_2", Email: "two@example.com", DisplayName: "Two", IsFirstUser: true, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, second); !errors.Is(err, ErrFirstUserExists) {
		t.Errorf("second CreateUser() error = %v, want ErrFirstUserExists", err)
	}

	// The rejected insert must not have left the second user behind under
	// a different constraint failure path.
	if _, err := db.GetUserByEmail(ctx, "two@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("GetUserByEmail(two@example.com) error = %v, want ErrUserNotFound", err)
	}
}

func TestUpdateUserPasswordHash(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u := User{ID: "user_1", Email: "a@example.com", DisplayName: "A", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	newHash := "new-hash"
	if err := db.UpdateUserPasswordHash(ctx, u.ID, &newHash); err != nil {
		t.Fatalf("UpdateUserPasswordHash() error = %v", err)
	}
	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.PasswordHash == nil || *got.PasswordHash != newHash {
		t.Errorf("PasswordHash = %v, want %q", got.PasswordHash, newHash)
	}

	if err := db.UpdateUserPasswordHash(ctx, u.ID, nil); err != nil {
		t.Fatalf("UpdateUserPasswordHash(nil) error = %v", err)
	}
	got, err = db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.PasswordHash != nil {
		t.Errorf("PasswordHash = %v, want nil after clearing", got.PasswordHash)
	}
}

func TestUpdateUserLastLogin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u := User{ID: "user_1", Email: "a@example.com", DisplayName: "A", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	when := time.Now().UTC().Truncate(time.Second)
	if err := db.UpdateUserLastLogin(ctx, u.ID, when); err != nil {
		t.Fatalf("UpdateUserLastLogin() error = %v", err)
	}
	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(when) {
		t.Errorf("LastLoginAt = %v, want %v", got.LastLoginAt, when)
	}
}

func TestListUsers_OrderedByCreatedAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	base := time.Now().UTC()
	for i, id := range []string{"user_1", "user_2", "user_3"} {
		u := User{
			ID:          id,
			Email:       id + "@example.com",
			DisplayName: id,
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser(%s) error = %v", id, err)
		}
	}

	got, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(ListUsers()) = %d, want 3", len(got))
	}
	for i, id := range []string{"user_1", "user_2", "user_3"} {
		if got[i].ID != id {
			t.Errorf("ListUsers()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestCountUsers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	n, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("CountUsers() = %d, want 0", n)
	}

	if err := db.CreateUser(ctx, User{ID: "user_1", Email: "a@example.com", DisplayName: "A", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	n, err = db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if n != 1 {
		t.Errorf("CountUsers() = %d, want 1", n)
	}
}

func TestDeleteUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u := User{ID: "user_1", Email: "a@example.com", DisplayName: "A", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := db.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if _, err := db.GetUserByID(ctx, u.ID); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("GetUserByID() after delete error = %v, want ErrUserNotFound", err)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.DeleteUser(ctx, "does-not-exist"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("DeleteUser() error = %v, want ErrUserNotFound", err)
	}
}

// TestMigration_LegacyAdminUserBecomesFirstUser proves migrations/0035's
// admin_user -> users backfill against the real migration SQL: applies
// every migration before 0030, seeds a legacy admin_user row with a
// bare username (no "@"), then applies 0030 and checks the result.
func TestMigration_LegacyAdminUserBecomesFirstUser(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	sqlDB, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db := &DB{DB: sqlDB}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}

	var usersMigration *migration
	for i := range migrations {
		if migrations[i].name == "users" {
			usersMigration = &migrations[i]
			break
		}
	}
	if usersMigration == nil {
		t.Fatal("users migration not found")
	}
	for i := range migrations {
		m := migrations[i]
		if m.version >= usersMigration.version {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			t.Fatalf("apply migration %d: %v", m.version, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO admin_user (id, username, password_hash, updated_at)
		VALUES (1, 'admin', 'bcrypt-hash-value', '2026-01-01T00:00:00.000000000Z')
	`); err != nil {
		t.Fatalf("seed legacy admin_user: %v", err)
	}

	if err := db.applyMigration(ctx, *usersMigration); err != nil {
		t.Fatalf("apply users migration: %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByEmail(admin) error = %v", err)
	}
	if !got.IsFirstUser {
		t.Errorf("IsFirstUser = false, want true for the migrated legacy admin")
	}
	if got.PasswordHash == nil || *got.PasswordHash != "bcrypt-hash-value" {
		t.Errorf("PasswordHash = %v, want the migrated bcrypt hash", got.PasswordHash)
	}
	if got.ID != "user_legacy_admin" {
		t.Errorf("ID = %q, want %q", got.ID, "user_legacy_admin")
	}
}

// TestMigration_NoLegacyAdmin_NoFirstUser proves the backfill is a
// genuine no-op on a fresh install: no admin_user row means no row
// inserted into users, leaving the very first real registration free to
// claim IsFirstUser via ux_users_single_first_user.
func TestMigration_NoLegacyAdmin_NoFirstUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.getFirstUser(ctx); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("fresh database already has a first user: %v", err)
	}
}
