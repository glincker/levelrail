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

func TestPooler_ImageUnsetUsesPinnedDefault(t *testing.T) {
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
	if cond, _ := poolerCond(t, res); cond.Reason != "PoolerRunning" {
		t.Errorf("pooler reason = %q, want PoolerRunning", cond.Reason)
	}
	if got := PoolerImage(); got != DefaultPoolerImage || strings.HasSuffix(got, ":latest") || !strings.Contains(got, ":v") {
		t.Errorf("PoolerImage() = %q, want pinned default", got)
	}
}

func TestPooler_ReplacedWhenPasswordRotates(t *testing.T) {
	t.Setenv(EnvPoolerImage, "example/pooler:1.0.0")
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", PoolerEnabled: true}
	creds := &PostgresCredentials{Username: "main", Password: "old-pw"}
	c := New("main", &fakeStore{db: desired}, rt, WithPostgresCredentials(creds))
	ctx := context.Background()
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	creates := rt.createCalls
	creds.Password = "new-pw"
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() after rotation error = %v", err)
	}
	if rt.createCalls <= creates {
		t.Error("pooler kept running with the old password after rotation")
	}
	for k, v := range rt.lastCreateSpec.Labels {
		if strings.Contains(v, "new-pw") {
			t.Errorf("label %s carries the password", k)
		}
	}
}

func TestParseShowPools(t *testing.T) {
	out := "database|user|cl_active|cl_waiting|cl_active_cancel_req|sv_active|maxwait|maxwait_us|pool_mode\n" +
		"app|app|10|3|0|10|7|500000|transaction\n" +
		"app|other|1|2|0|1|2|0|transaction\n" +
		"pgbouncer|pgbouncer|1|0|0|0|0|0|statement\n" +
		"(3 rows)\n"
	got, err := ParseShowPools(out)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientsWaiting != 5 || got.MaxWaitSeconds != 7.5 {
		t.Fatalf("got %+v, want 5 waiting and 7.5s", got)
	}
	if _, err := ParseShowPools("psql: error: connection refused"); err == nil {
		t.Fatal("expected an error for output without a SHOW POOLS header")
	}
}
