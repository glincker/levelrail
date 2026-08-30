package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDatabaseStore is a hand-written fake for DatabaseAttachmentStore,
// the same pattern fakeStorageTargetStore already establishes for
// StorageTargetStore.
type fakeDatabaseStore struct {
	databases map[string]store.DesiredDatabase
	calls     int
}

func (f *fakeDatabaseStore) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	f.calls++
	db, ok := f.databases[name]
	if !ok {
		return nil, store.ErrDatabaseNotFound
	}
	return &db, nil
}

func TestController_Reconcile_NoDatabaseEnv_Unaffected(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env: map[string]string{"NODE_ENV": "production"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if conditionOf(t, result).Status != reconcile.ConditionTrue {
		t.Errorf("condition status = %v, want True", conditionOf(t, result).Status)
	}
	if dbStore.calls != 0 {
		t.Errorf("GetDesiredDatabase calls = %d, want 0: a service with no database-backed env var must never look one up", dbStore.calls)
	}
	if got := rt.lastCreateEnv["NODE_ENV"]; got != "production" {
		t.Errorf("container env NODE_ENV = %q, want the literal value preserved", got)
	}
}

func TestController_Reconcile_DatabaseEnv_Postgres_URL_Injected(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"main/" + database.PostgresPasswordEnvKey: "s3cr3t",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore), WithSecretResolver(secretResolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if conditionOf(t, result).Status != reconcile.ConditionTrue {
		t.Errorf("condition status = %v, want True", conditionOf(t, result).Status)
	}
	want := "postgres://main:s3cr3t@db-main:5432/main" //nolint:gosec // fake fixture, not a real credential
	if got := rt.lastCreateEnv["DATABASE_URL"]; got != want {
		t.Errorf("container env DATABASE_URL = %q, want %q", got, want)
	}
}

func TestController_Reconcile_DatabaseEnv_PerFieldVariants(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EngineMySQL},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"main/" + database.MySQLPasswordEnvKey: "hunter2",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DB_HOST":     {Database: "main", Field: "host"},
			"DB_PORT":     {Database: "main", Field: "port"},
			"DB_USER":     {Database: "main", Field: "username"},
			"DB_PASSWORD": {Database: "main", Field: "password"},
			"DB_NAME":     {Database: "main", Field: "database"},
		},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore), WithSecretResolver(secretResolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	want := map[string]string{
		"DB_HOST":     "db-main",
		"DB_PORT":     "3306",
		"DB_USER":     "main",
		"DB_PASSWORD": "hunter2",
		"DB_NAME":     "main",
	}
	for k, wantV := range want {
		if gotV := rt.lastCreateEnv[k]; gotV != wantV {
			t.Errorf("container env %s = %q, want %q", k, gotV, wantV)
		}
	}
}

// TestController_Reconcile_DatabaseEnv_RedisProtocolFamily_URL_UsesRedisScheme
// guards resolveDatabaseURL's Redis-protocol-family branch: all three
// engines are passwordless drop-in Redis forks, so their URL must use
// the "redis://" scheme a Redis client library actually recognizes, not
// "keydb://"/"dragonfly://" (neither KeyDB nor Dragonfly has its own
// PasswordSecretKey entry, so falling through to the generic
// credentialed branch would fail resolution entirely, the bug this test
// locks in the fix for).
func TestController_Reconcile_DatabaseEnv_RedisProtocolFamily_URL_UsesRedisScheme(t *testing.T) {
	tests := []struct {
		name   string
		engine string
	}{
		{name: "Redis", engine: store.EngineRedis},
		{name: "KeyDB", engine: store.EngineKeyDB},
		{name: "Dragonfly", engine: store.EngineDragonfly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
				"cache": {Name: "cache", Engine: tt.engine},
			}}
			desired := &store.DesiredService{
				Name: "web", Image: "img:v1", Port: 80,
				DatabaseEnv: map[string]store.DatabaseEnvRef{
					"CACHE_URL": {Database: "cache", Field: "url"},
				},
			}
			// No WithSecretResolver: neither engine needs one, so this
			// must still succeed.
			c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore))

			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			want := "redis://db-cache:6379"
			if got := rt.lastCreateEnv["CACHE_URL"]; got != want {
				t.Errorf("container env CACHE_URL = %q, want %q", got, want)
			}
		})
	}
}

// TestController_Reconcile_DatabaseEnv_UnknownDatabase_FailsLoudly is the
// case a { from: ... } env var referencing a database that doesn't (or
// no longer) exist must fail Reconcile clearly, not silently start a
// container with the variable missing or empty.
func TestController_Reconcile_DatabaseEnv_UnknownDatabase_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{}} // empty: "main" doesn't exist
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a reference to a nonexistent database to fail loudly")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a container must never be created missing a database env var it declared", rt.createCalls)
	}
}

func TestController_Reconcile_DatabaseEnv_NoDatabaseStoreConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	// No WithDatabaseAttachments.
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a database-backed env var with no store configured to fail loudly")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_DatabaseEnv_NoSecretResolverConfigured_FailsLoudly(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	// No WithSecretResolver: postgres needs one to resolve its password.
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a postgres url reference with no secret resolver to fail loudly")
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_DatabaseAttachment_Injected(t *testing.T) {
	rt := newFakeRuntime(0)
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"main/" + database.PostgresPasswordEnvKey: "s3cr3t",
	})
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseAttachment: &store.DatabaseAttachment{DatabaseName: "main", EnvVar: "DATABASE_URL", Field: "url"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore), WithSecretResolver(secretResolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	want := "postgres://main:s3cr3t@db-main:5432/main" //nolint:gosec // fake fixture, not a real credential
	if got := rt.lastCreateEnv["DATABASE_URL"]; got != want {
		t.Errorf("container env DATABASE_URL = %q, want %q", got, want)
	}
}
