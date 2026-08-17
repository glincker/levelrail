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

func TestMySQLCredentialsFor_NilSecretsManager_ReturnsNil(t *testing.T) {
	creds, err := mysqlCredentialsFor(context.Background(), nil, "main")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor() error = %v", err)
	}
	if creds != nil {
		t.Errorf("creds = %+v, want nil (no secrets manager configured)", creds)
	}
}

func TestMySQLCredentialsFor_GeneratesOnFirstCall(t *testing.T) {
	mgr := newTestSecretsManager(t)

	creds, err := mysqlCredentialsFor(context.Background(), mgr, "main")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor() error = %v", err)
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

func TestMySQLCredentialsFor_ReusesExistingPasswordOnSecondCall(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	first, err := mysqlCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}

	second, err := mysqlCredentialsFor(ctx, mgr, "main")
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

func TestMySQLCredentialsFor_DifferentDatabasesGetDifferentPasswords(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	main, err := mysqlCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor(main) error = %v", err)
	}
	analytics, err := mysqlCredentialsFor(ctx, mgr, "analytics")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor(analytics) error = %v", err)
	}

	if main.Password == analytics.Password {
		t.Error("two distinct databases got the same generated password")
	}
}

func TestMySQLCredentialsFor_SamedDatabaseName_DistinctFromPostgresCredentials(t *testing.T) {
	// mysqlCredentialsFor and postgresCredentialsFor must use distinct
	// secrets-store keys for the same dbName: a database named "main"
	// could theoretically exist as one engine now and be recreated as
	// the other later (or, more realistically, this just proves the two
	// helpers don't accidentally share storage and silently overwrite
	// each other's password for the same name).
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	pg, err := postgresCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("postgresCredentialsFor(main) error = %v", err)
	}
	mysql, err := mysqlCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor(main) error = %v", err)
	}

	if pg.Password == mysql.Password {
		t.Error("postgres and mysql credentials for the same database name share a password, storage keys must not collide")
	}
}

func TestMongoCredentialsFor_NilSecretsManager_ReturnsNil(t *testing.T) {
	creds, err := mongoCredentialsFor(context.Background(), nil, "main")
	if err != nil {
		t.Fatalf("mongoCredentialsFor() error = %v", err)
	}
	if creds != nil {
		t.Errorf("creds = %+v, want nil (no secrets manager configured)", creds)
	}
}

func TestMongoCredentialsFor_GeneratesOnFirstCall(t *testing.T) {
	mgr := newTestSecretsManager(t)

	creds, err := mongoCredentialsFor(context.Background(), mgr, "main")
	if err != nil {
		t.Fatalf("mongoCredentialsFor() error = %v", err)
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

func TestMongoCredentialsFor_ReusesExistingPasswordOnSecondCall(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	first, err := mongoCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}

	second, err := mongoCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}

	if first.Password != second.Password {
		t.Errorf("Password changed between calls: %q != %q, a reconcile pass must never rotate a running database's password out from under it", first.Password, second.Password)
	}
}

func TestMongoCredentialsFor_SameDatabaseName_DistinctFromOtherEngines(t *testing.T) {
	mgr := newTestSecretsManager(t)
	ctx := context.Background()

	mongo, err := mongoCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("mongoCredentialsFor(main) error = %v", err)
	}
	mysql, err := mysqlCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("mysqlCredentialsFor(main) error = %v", err)
	}
	pg, err := postgresCredentialsFor(ctx, mgr, "main")
	if err != nil {
		t.Fatalf("postgresCredentialsFor(main) error = %v", err)
	}

	if mongo.Password == mysql.Password || mongo.Password == pg.Password {
		t.Error("mongodb credentials for a database name share a password with another engine's, storage keys must not collide")
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
