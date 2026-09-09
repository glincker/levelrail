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

// newDatabaseEnvController builds a Controller for the given desired
// service against dbStore, optionally wiring a fake secret resolver from
// secrets (nil means no resolver configured at all, the "no secrets
// manager" case some tests deliberately exercise).
func newDatabaseEnvController(desired *store.DesiredService, dbStore *fakeDatabaseStore, secrets map[string]string) (*Controller, *fakeRuntime) {
	rt := newFakeRuntime(0)
	var opts []Option
	if dbStore != nil {
		opts = append(opts, WithDatabaseAttachments(dbStore))
	}
	if secrets != nil {
		opts = append(opts, WithSecretResolver(newFakeSecretResolver(secrets)))
	}
	return New("web", &fakeStore{svc: desired}, rt, opts...), rt
}

// reconcileAndAssertEnv reconciles c and checks each wanted key/value pair
// against the container env the fake runtime actually received.
func reconcileAndAssertEnv(t *testing.T, c *Controller, rt *fakeRuntime, want map[string]string) {
	t.Helper()
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for k, wantV := range want {
		if got := rt.lastCreateEnv[k]; got != wantV {
			t.Errorf("container env %s = %q, want %q", k, got, wantV)
		}
	}
}

// assertDatabaseEnvReconcileFailsLoudly reconciles c and requires it to
// fail with a False condition and zero container creates: the shared shape
// every "misconfigured database env" test in this file must prove.
func assertDatabaseEnvReconcileFailsLoudly(t *testing.T, c *Controller, rt *fakeRuntime, wantErrMsg string) {
	t.Helper()
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatalf("Reconcile() error = nil, want %s", wantErrMsg)
	}
	if conditionOf(t, result).Status != reconcile.ConditionFalse {
		t.Errorf("condition status = %v, want False", conditionOf(t, result).Status)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Reconcile_NoDatabaseEnv_Unaffected(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Env: map[string]string{"NODE_ENV": "production"},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, nil)

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
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{
		"main/" + database.PostgresPasswordEnvKey: "s3cr3t",
	})

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
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EngineMySQL},
	}}
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
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{
		"main/" + database.MySQLPasswordEnvKey: "hunter2",
	})

	reconcileAndAssertEnv(t, c, rt, map[string]string{
		"DB_HOST":     "db-main",
		"DB_PORT":     "3306",
		"DB_USER":     "main",
		"DB_PASSWORD": "hunter2",
		"DB_NAME":     "main",
	})
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
			dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
				"cache": {Name: "cache", Engine: tt.engine},
			}}
			desired := &store.DesiredService{
				Name: "web", Image: "img:v1", Port: 80,
				DatabaseEnv: map[string]store.DatabaseEnvRef{
					"CACHE_URL": {Database: "cache", Field: "url"},
				},
			}
			// No secrets: neither engine needs one, so this must still succeed.
			c, rt := newDatabaseEnvController(desired, dbStore, nil)

			reconcileAndAssertEnv(t, c, rt, map[string]string{"CACHE_URL": "redis://db-cache:6379"})
		})
	}
}

// TestController_Reconcile_DatabaseEnv_UnknownDatabase_FailsLoudly is the
// case a { from: ... } env var referencing a database that doesn't (or
// no longer) exist must fail Reconcile clearly, not silently start a
// container with the variable missing or empty.
func TestController_Reconcile_DatabaseEnv_UnknownDatabase_FailsLoudly(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{}} // empty: "main" doesn't exist
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, nil)

	assertDatabaseEnvReconcileFailsLoudly(t, c, rt, "a reference to a nonexistent database to fail loudly")
}

func TestController_Reconcile_DatabaseEnv_NoDatabaseStoreConfigured_FailsLoudly(t *testing.T) {
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	// No dbStore: WithDatabaseAttachments is deliberately left unconfigured.
	c, rt := newDatabaseEnvController(desired, nil, nil)

	assertDatabaseEnvReconcileFailsLoudly(t, c, rt, "a database-backed env var with no store configured to fail loudly")
}

func TestController_Reconcile_DatabaseEnv_NoSecretResolverConfigured_FailsLoudly(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}
	// No secrets: postgres needs a resolver to resolve its password.
	c, rt := newDatabaseEnvController(desired, dbStore, nil)

	assertDatabaseEnvReconcileFailsLoudly(t, c, rt, "a postgres url reference with no secret resolver to fail loudly")
}

// TestController_Reconcile_DatabaseEnv_Postgres_TLS_URL_HasSSLMode proves
// resolveDatabaseURL appends "?sslmode=require" once this database's TLS
// certificate exists (database.TLSCertEnvKey, cmd/levelrail's
// tlsMaterialFor), the same "encrypt without verifying" contract Postgres
// client libraries already honor with zero app-side code changes.
func TestController_Reconcile_DatabaseEnv_Postgres_TLS_URL_HasSSLMode(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
			"DB_PORT":      {Database: "main", Field: "port"},
		},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{
		"main/" + database.PostgresPasswordEnvKey: "s3cr3t",
		"main/" + database.TLSCertEnvKey:          "-----BEGIN CERTIFICATE-----\n...",
	})

	// Postgres negotiates TLS on its one existing port: unlike Redis,
	// TLS must never change the port field.
	reconcileAndAssertEnv(t, c, rt, map[string]string{ //nolint:gosec // fake fixture, not a real credential
		"DATABASE_URL": "postgres://main:s3cr3t@db-main:5432/main?sslmode=require",
		"DB_PORT":      "5432",
	})
}

// TestController_Reconcile_DatabaseEnv_Redis_TLS_URL_UsesRedissSchemeAndTLSPort
// proves Redis's TLS-enabled connection info flips both the scheme
// ("rediss://") and the port (WithTLS disables Redis's plaintext port
// entirely server-side, internal/reconcile/database's own
// redisCommandAndPort doc comment), not just the scheme: a client
// dialing the old plaintext port after this would get nothing.
func TestController_Reconcile_DatabaseEnv_Redis_TLS_URL_UsesRedissSchemeAndTLSPort(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"cache": {Name: "cache", Engine: store.EngineRedis},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"CACHE_URL":  {Database: "cache", Field: "url"},
			"CACHE_PORT": {Database: "cache", Field: "port"},
		},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{
		"cache/" + database.TLSCertEnvKey: "-----BEGIN CERTIFICATE-----\n...",
	})

	reconcileAndAssertEnv(t, c, rt, map[string]string{
		"CACHE_URL":  "rediss://db-cache:6380",
		"CACHE_PORT": "6380",
	})
}

// TestController_Reconcile_DatabaseEnv_TLSNotYetGenerated_StaysPlaintext
// proves a database whose engine SupportsTLS but has no TLS certificate
// generated yet (no secrets master key configured, or not reconciled
// since this feature landed) resolves exactly as it did before this
// feature existed: no query param, the plaintext port, "redis://" not
// "rediss://". This is what keeps an existing, already-running database
// on its original connection string until it's genuinely recreated (see
// internal/reconcile/database's WithTLS doc comment).
func TestController_Reconcile_DatabaseEnv_TLSNotYetGenerated_StaysPlaintext(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"cache": {Name: "cache", Engine: store.EngineRedis},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"CACHE_URL": {Database: "cache", Field: "url"},
		},
	}
	// Non-nil, empty secrets: a resolver is configured but has no
	// TLSCertEnvKey entry, since TLS material was never generated.
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{})

	reconcileAndAssertEnv(t, c, rt, map[string]string{"CACHE_URL": "redis://db-cache:6379"})
}

func TestController_Reconcile_DatabaseAttachment_Injected(t *testing.T) {
	dbStore := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseAttachment: &store.DatabaseAttachment{DatabaseName: "main", EnvVar: "DATABASE_URL", Field: "url"},
	}
	c, rt := newDatabaseEnvController(desired, dbStore, map[string]string{
		"main/" + database.PostgresPasswordEnvKey: "s3cr3t",
	})

	want := "postgres://main:s3cr3t@db-main:5432/main" //nolint:gosec // fake fixture, not a real credential
	reconcileAndAssertEnv(t, c, rt, map[string]string{"DATABASE_URL": want})
}
