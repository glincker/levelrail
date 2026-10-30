package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeScopeStore struct{ scopes map[string]string }

func (f fakeScopeStore) GetDatabaseAccessSettings(_ context.Context, name string) (store.DatabaseAccessSettings, error) {
	return store.DatabaseAccessSettings{Database: name, Scope: f.scopes[name]}, nil
}

func (f fakeScopeStore) EnvironmentOfDatabase(context.Context, string) (*store.EnvironmentRef, error) {
	return nil, nil
}

func TestConnectReferencedDatabases_ContinuesPastOutOfScope(t *testing.T) {
	dbs := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{
		"a": {Name: "a", Engine: store.EnginePostgres, ProjectID: "other"},
		"b": {Name: "b", Engine: store.EnginePostgres, ProjectID: "p1"},
		"c": {Name: "c", Engine: store.EnginePostgres, ProjectID: "p1"},
	}}
	scopes := fakeScopeStore{scopes: map[string]string{"a": "project", "b": "project", "c": "platform"}}
	desired := &store.DesiredService{
		Name: "web", AppID: "app-1", ProjectID: "p1",
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"A_URL": {Database: "a", Field: "url"},
			"B_URL": {Database: "b", Field: "url"},
			"C_URL": {Database: "c", Field: "url"},
		},
	}
	rt := newFakeRuntime(0)
	for _, n := range []string{"a", "b", "c"} {
		rt.seed(database.ContainerName(n), true)
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithDatabaseAttachments(dbs), WithDatabaseScope(scopes))

	err := c.connectReferencedDatabases(context.Background(), desired, "net-1")
	if !errors.Is(err, ErrDatabaseOutOfScope) {
		t.Fatalf("err = %v, want ErrDatabaseOutOfScope", err)
	}
	want := map[string]bool{"net-1:" + database.ContainerName("b"): true, "net-1:" + database.ContainerName("c"): true}
	if len(rt.connections) != len(want) {
		t.Fatalf("connections = %v, want b and c only", rt.connections)
	}
	for _, conn := range rt.connections {
		if !want[conn] {
			t.Errorf("unexpected connection %q", conn)
		}
	}
}
