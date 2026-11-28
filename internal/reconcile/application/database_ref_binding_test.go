package application

import (
	"context"
	"errors"
	"testing"

	"github.com/glincker/levelrail/internal/reconcile/database"
	"github.com/glincker/levelrail/internal/store"
)

type ownerFakeStore struct {
	eventFakeStore
	apps, dbs map[string]string
}

func (s *ownerFakeStore) OwnerOfApp(_ context.Context, name string) (*store.ResourceOwner, error) {
	return &store.ResourceOwner{TeamID: s.apps[name]}, nil
}

func (s *ownerFakeStore) OwnerOfDatabase(_ context.Context, name string) (*store.ResourceOwner, error) {
	return &store.ResourceOwner{TeamID: s.dbs[name]}, nil
}

func bindingController(st *ownerFakeStore, live store.DesiredDatabase) *Controller {
	dbs := &fakeDatabaseStore{databases: map[string]store.DesiredDatabase{live.Name: live}}
	secrets := newFakeSecretResolver(map[string]string{live.Name + "/" + database.PostgresPasswordEnvKey: "team-b-password"})
	return New("spy", st, newFakeRuntime(0), WithDatabaseAttachments(dbs), WithSecretResolver(secrets))
}

func resolveOrders(t *testing.T, c *Controller, ref store.DatabaseEnvRef, att *store.DatabaseAttachment) (map[string]string, error) {
	t.Helper()
	desired := &store.DesiredService{Name: "spy", DatabaseAttachment: att}
	if att == nil {
		desired.DatabaseEnv = map[string]store.DatabaseEnvRef{"DB": ref}
	}
	return c.resolveDatabaseEnv(context.Background(), desired)
}

// A force deleted database whose name another team reuses never hands that team's credentials to the old app.
func TestDatabaseRefRefusedForRecreatedDatabase(t *testing.T) {
	st := &ownerFakeStore{apps: map[string]string{"spy": "team-a"}, dbs: map[string]string{"orders": "team-b"}}
	c := bindingController(st, store.DesiredDatabase{Name: "orders", Engine: store.EnginePostgres, InstanceID: "inst-b"})

	env, err := resolveOrders(t, c, store.DatabaseEnvRef{Database: "orders", Field: "url", Instance: "inst-a"}, nil)
	if !errors.Is(err, errDatabaseRefRecreated) || len(env) != 0 {
		t.Fatalf("env = %v, err = %v, want errDatabaseRefRecreated and no env", env, err)
	}
	for i := 0; i < 3; i++ {
		_, _ = resolveOrders(t, c, store.DatabaseEnvRef{Database: "orders", Field: "url", Instance: "inst-a"}, nil)
	}
	if len(st.events) != 1 || st.events[0].Kind != store.AppEventDatabaseRefRefused {
		t.Fatalf("events = %+v, want one coalesced database_ref_refused event", st.events)
	}
}

// The same team recreating its own database must re-attach explicitly, and a dangling marker never resolves.
func TestDatabaseRefRefusedForSameTeamRecreateAndDangling(t *testing.T) {
	st := &ownerFakeStore{apps: map[string]string{"spy": "team-a"}, dbs: map[string]string{"orders": "team-a"}}
	c := bindingController(st, store.DesiredDatabase{Name: "orders", Engine: store.EnginePostgres, InstanceID: "inst-2"})

	for _, instance := range []string{"inst-1", store.DanglingDatabaseInstance} {
		if _, err := resolveOrders(t, c, store.DatabaseEnvRef{Database: "orders", Field: "url", Instance: instance}, nil); !errors.Is(err, errDatabaseRefRecreated) {
			t.Fatalf("instance %q err = %v, want errDatabaseRefRecreated", instance, err)
		}
	}
	att := &store.DatabaseAttachment{DatabaseName: "orders", EnvVar: "DB", Field: "url", InstanceID: "inst-1"}
	if _, err := resolveOrders(t, c, store.DatabaseEnvRef{}, att); !errors.Is(err, errDatabaseRefRecreated) {
		t.Fatalf("attachment err = %v, want errDatabaseRefRecreated", err)
	}
}

// A ref bound to the live instance resolves; an unbound legacy ref resolves only inside one team.
func TestDatabaseRefBoundAndLegacy(t *testing.T) {
	live := store.DesiredDatabase{Name: "orders", Engine: store.EnginePostgres, InstanceID: "inst-1"}
	same := &ownerFakeStore{apps: map[string]string{"spy": "team-a"}, dbs: map[string]string{"orders": "team-a"}}
	other := &ownerFakeStore{apps: map[string]string{"spy": "team-a"}, dbs: map[string]string{"orders": "team-b"}}

	if env, err := resolveOrders(t, bindingController(same, live), store.DatabaseEnvRef{Database: "orders", Field: "url", Instance: "inst-1"}, nil); err != nil || env["DB"] == "" {
		t.Fatalf("bound ref env = %v, err = %v, want resolved", env, err)
	}
	if env, err := resolveOrders(t, bindingController(same, live), store.DatabaseEnvRef{Database: "orders", Field: "url"}, nil); err != nil || env["DB"] == "" {
		t.Fatalf("legacy same team env = %v, err = %v, want resolved", env, err)
	}
	env, err := resolveOrders(t, bindingController(other, live), store.DatabaseEnvRef{Database: "orders", Field: "url"}, nil)
	if !errors.Is(err, errDatabaseRefTeam) || len(env) != 0 {
		t.Fatalf("legacy other team env = %v, err = %v, want errDatabaseRefTeam", env, err)
	}
	if len(other.events) != 1 {
		t.Fatalf("events = %+v, want one stored security event", other.events)
	}
}
