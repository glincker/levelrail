package firewall

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/firewall"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	services  []store.DesiredService
	databases []store.DesiredDatabase
	listErr   error
}

func (f *fakeStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.services, nil
}

func (f *fakeStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return f.databases, nil
}

type fakeSyncer struct {
	gotWant []firewall.Rule
	result  firewall.Result
	err     error
}

func (f *fakeSyncer) Sync(_ context.Context, want []firewall.Rule) (firewall.Result, error) {
	f.gotWant = want
	return f.result, f.err
}

func hostPort(p int) *int { return &p }

func TestController_Reconcile_BuildsWantFromLocalResourcesOnly(t *testing.T) {
	st := &fakeStore{
		services: []store.DesiredService{
			{Name: "web", HostPort: hostPort(8080), NodeID: ""},
			{Name: "no-port"},
			{Name: "remote-web", HostPort: hostPort(9999), NodeID: "node-2"},
		},
		databases: []store.DesiredDatabase{
			{Name: "mydb", PubliclyAccessible: true, PublicPort: 20000, NodeID: ""},
			{Name: "private-db", PubliclyAccessible: false},
			{Name: "remote-db", PubliclyAccessible: true, PublicPort: 20001, NodeID: "node-2"},
		},
	}
	syncer := &fakeSyncer{result: firewall.Result{Installed: true, Active: true}}
	c := New(st, syncer)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(syncer.gotWant) != 2 {
		t.Fatalf("Sync() called with %d rules, want 2 (remote-node resources must be excluded): %+v", len(syncer.gotWant), syncer.gotWant)
	}
	byOwner := map[string]int{}
	for _, r := range syncer.gotWant {
		byOwner[r.Owner] = r.Port
	}
	if byOwner["app:web"] != 8080 {
		t.Errorf("app:web port = %d, want 8080", byOwner["app:web"])
	}
	if byOwner["db:mydb"] != 20000 {
		t.Errorf("db:mydb port = %d, want 20000", byOwner["db:mydb"])
	}
	if _, ok := byOwner["app:remote-web"]; ok {
		t.Error("remote-web (a different node) should never reach Sync's want list")
	}
	if _, ok := byOwner["db:remote-db"]; ok {
		t.Error("remote-db (a different node) should never reach Sync's want list")
	}

	if len(result.Conditions) != 1 || result.Conditions[0].Status != reconcile.ConditionTrue {
		t.Fatalf("Reconcile() result = %+v, want a single True condition", result)
	}
	if result.Conditions[0].Reason != "Synced2Rules" {
		t.Errorf("Reason = %q, want Synced2Rules", result.Conditions[0].Reason)
	}
}

func TestController_Reconcile_ListError(t *testing.T) {
	st := &fakeStore{listErr: errors.New("boom")}
	c := New(st, &fakeSyncer{})

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a store error")
	}
	if len(result.Conditions) != 1 || result.Conditions[0].Status != reconcile.ConditionFalse || result.Conditions[0].Reason != "StoreError" {
		t.Errorf("result = %+v, want a False/StoreError condition", result)
	}
}

func TestController_Reconcile_UFWNotInstalled(t *testing.T) {
	c := New(&fakeStore{}, &fakeSyncer{result: firewall.Result{Installed: false}})
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (ufw missing is not a failure)", err)
	}
	if result.Conditions[0].Status != reconcile.ConditionUnknown || result.Conditions[0].Reason != "UFWNotInstalled" {
		t.Errorf("result = %+v, want Unknown/UFWNotInstalled", result)
	}
}

func TestController_Reconcile_UFWInactive(t *testing.T) {
	c := New(&fakeStore{}, &fakeSyncer{result: firewall.Result{Installed: true, Active: false}})
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (ufw inactive is not a failure)", err)
	}
	if result.Conditions[0].Status != reconcile.ConditionUnknown || result.Conditions[0].Reason != "UFWInactive" {
		t.Errorf("result = %+v, want Unknown/UFWInactive", result)
	}
}

func TestController_Reconcile_PartialSyncFailureIsReportedNotErrored(t *testing.T) {
	st := &fakeStore{services: []store.DesiredService{{Name: "web", HostPort: hostPort(8080)}}}
	syncer := &fakeSyncer{result: firewall.Result{Installed: true, Active: true, Errors: []string{"allow 8080/tcp: boom"}}}
	c := New(st, syncer)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (a partial sync failure is reported via the condition, not a reconcile error)", err)
	}
	if result.Conditions[0].Status != reconcile.ConditionFalse || result.Conditions[0].Reason != "RuleSyncFailed" {
		t.Errorf("result = %+v, want False/RuleSyncFailed", result)
	}
}

func TestController_Reconcile_SyncError(t *testing.T) {
	c := New(&fakeStore{}, &fakeSyncer{err: errors.New("exec failed")})
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a sync error")
	}
	if result.Conditions[0].Reason != "SyncFailed" {
		t.Errorf("Reason = %q, want SyncFailed", result.Conditions[0].Reason)
	}
}

func TestController_Name(t *testing.T) {
	c := New(&fakeStore{}, &fakeSyncer{})
	if c.Name() != "firewall" {
		t.Errorf("Name() = %q, want %q", c.Name(), "firewall")
	}
}
