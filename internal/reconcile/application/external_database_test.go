package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type externalAwareStore struct {
	fakeDatabaseStore
	external map[string]store.ExternalDatabase
}

func (f *externalAwareStore) GetExternalDatabase(_ context.Context, name string) (*store.ExternalDatabase, error) {
	d, ok := f.external[name]
	if !ok {
		return nil, store.ErrExternalDatabaseNotFound
	}
	return &d, nil
}

func TestController_Reconcile_ExternalDatabaseEnvResolves(t *testing.T) {
	dbStore := &externalAwareStore{
		fakeDatabaseStore: fakeDatabaseStore{databases: map[string]store.DesiredDatabase{}},
		external: map[string]store.ExternalDatabase{
			"legacy": {Name: "legacy", Engine: store.EnginePostgres, Host: "coolify-pg", Port: 5432, Username: "app", DatabaseName: "appdb", TLSMode: "prefer", Network: "coolify"},
		},
	}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "legacy", Field: "url"},
			"PGHOST":       {Database: "legacy", Field: "host"},
			"PGPORT":       {Database: "legacy", Field: "port"},
			"PGUSER":       {Database: "legacy", Field: "username"},
			"PGDATABASE":   {Database: "legacy", Field: "database"},
		},
	}
	rt := newFakeRuntime(0)
	secrets := map[string]string{"external-database/legacy/password": "p@ss w0rd"}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbStore), WithSecretResolver(newFakeSecretResolver(secrets)))

	reconcileAndAssertEnv(t, c, rt, map[string]string{ //nolint:gosec // fixture credentials
		"DATABASE_URL": "postgres://app:p%40ss%20w0rd@coolify-pg:5432/appdb?sslmode=prefer",
		"PGHOST":       "coolify-pg",
		"PGPORT":       "5432",
		"PGUSER":       "app",
		"PGDATABASE":   "appdb",
	})
}
