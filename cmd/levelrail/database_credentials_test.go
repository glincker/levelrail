package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func openCredentialsTestDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})
	return db
}

func newTestSecretsManager(t *testing.T) *secrets.Manager {
	t.Helper()
	db := openCredentialsTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	return secrets.NewManager(db, mk)
}

func TestPostgresCredentialsFor_NilSecretsManager_ReturnsNil(t *testing.T) {
	creds, err := postgresCredentialsFor(context.Background(), nil, "main")
	if err != nil {
		t.Fatalf("postgresCredentialsFor() error = %v", err)
	}
	if creds != nil {
		t.Errorf("creds = %+v, want nil (no secrets manager configured)", creds)
	}
}

func TestPostgresCredentialsFor_GeneratesOnFirstCall(t *testing.T) {
	mgr := newTestSecretsManager(t)

	creds, err := postgresCredentialsFor(context.Background(), mgr, "main")
	if err != nil {
		t.Fatalf("postgresCredentialsFor() error = %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want a real credentials value")
	}
	if creds.Username == "" {
		t.Error("Username is empty")
	}
	if creds.Password == "" {
		t.Error("Password is empty")
	}
}

func TestPostgresCredentialsFor_ReusesExistingPasswordOnSecondCall(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	first, err := postgresCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}

	second, err := postgresCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}

	if first.Password != second.Password {
		t.Errorf("Password changed between calls: %q != %q, a reconcile pass must never rotate a running database's password out from under it", first.Password, second.Password)
	}
	if first.Username != second.Username {
		t.Errorf("Username changed between calls: %q != %q", first.Username, second.Username)
	}
}

func TestPostgresCredentialsFor_DifferentDatabasesGetDifferentPasswords(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	main, err := postgresCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("postgresCredentialsFor(main) error = %v", err)
	}
	analytics, err := postgresCredentialsFor(ctx, mgr, "analytics")
	if err != nil {
		t.Fatalf("postgresCredentialsFor(analytics) error = %v", err)
	}

	if main.Password == analytics.Password {
		t.Error("two distinct databases got the same generated password")
	}
}

func TestGenerateRandomPassword_ReturnsNonEmptyUniqueValues(t *testing.T) {
	a, err := generateRandomPassword()
	if err != nil {
		t.Fatalf("generateRandomPassword() error = %v", err)
	}
	b, err := generateRandomPassword()
	if err != nil {
		t.Fatalf("generateRandomPassword() error = %v", err)
	}
	if a == "" {
		t.Error("generated password is empty")
	}
	if a == b {
		t.Error("two calls produced the same password")
	}
	if len(a) < 20 {
		t.Errorf("generated password len = %d, want a real, non-trivial length", len(a))
	}
}
