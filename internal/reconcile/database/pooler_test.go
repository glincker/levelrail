package database

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func poolerCond(t *testing.T, res reconcile.Result) (reconcile.Condition, bool) {
	t.Helper()
	for _, c := range res.Conditions {
		if c.Type == PoolerConditionType {
			return c, true
		}
	}
	return reconcile.Condition{}, false
}

func TestPooler_LifecycleFollowsDesiredState(t *testing.T) {
	t.Setenv(EnvPoolerImage, "example/pooler:1.0.0")
	t.Setenv(EnvPoolerDefaultPoolSize, "15")
	t.Setenv(EnvPoolerMaxClientConn, "not-a-number")
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", PoolerEnabled: true}
	c := New("main", &fakeStore{db: desired}, rt, WithPostgresCredentials(&PostgresCredentials{Username: "main", Password: "pw-value"}))
	ctx := context.Background()

	res, err := c.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond, ok := poolerCond(t, res); !ok || cond.Status != reconcile.ConditionTrue {
		t.Fatalf("pooler condition = %+v (found %v), want True", cond, ok)
	}
	db, _ := rt.InspectByName(ctx, ContainerName("main"))
	spec := rt.lastCreateSpec
	if spec.Name != PoolerContainerName("main") || spec.NetworkMode != "container:"+db.ID {
		t.Fatalf("pooler spec name=%q network=%q, want shared namespace of %s", spec.Name, spec.NetworkMode, db.ID)
	}
	if spec.Env["POOL_MODE"] != "transaction" || spec.Env["DEFAULT_POOL_SIZE"] != "15" {
		t.Errorf("pooler env pool settings wrong: mode=%q size=%q", spec.Env["POOL_MODE"], spec.Env["DEFAULT_POOL_SIZE"])
	}
	if _, ok := spec.Env["MAX_CLIENT_CONN"]; ok {
		t.Error("an invalid env value must fall back to the pooler default")
	}
	for k, v := range spec.Labels {
		if strings.Contains(v, "pw-value") {
			t.Errorf("label %s carries the password", k)
		}
	}

	creates := rt.createCalls
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if rt.createCalls != creates {
		t.Errorf("steady state recreated the pooler: creates %d -> %d", creates, rt.createCalls)
	}

	desired.PoolerEnabled = false
	res, err = c.Reconcile(ctx)
	if err != nil {
		t.Fatalf("disable Reconcile() error = %v", err)
	}
	if _, ok := poolerCond(t, res); ok {
		t.Error("a disabled pooler should report no condition")
	}
	if st, _ := rt.InspectByName(ctx, PoolerContainerName("main")); st != nil {
		t.Error("pooler container still present after disable")
	}
}

func TestPooler_ImageUnsetKeepsDatabaseReady(t *testing.T) {
	t.Setenv(EnvPoolerImage, "")
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", PoolerEnabled: true}
	c := New("main", &fakeStore{db: desired}, rt, WithPostgresCredentials(&PostgresCredentials{Username: "main", Password: "x"}))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if conditionOf(t, res).Status != reconcile.ConditionTrue {
		t.Errorf("database Ready = %+v, want True", conditionOf(t, res))
	}
	if cond, _ := poolerCond(t, res); cond.Reason != "PoolerImageNotConfigured" {
		t.Errorf("pooler reason = %q, want PoolerImageNotConfigured", cond.Reason)
	}
}
