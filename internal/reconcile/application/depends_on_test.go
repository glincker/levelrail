package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_DependencyBlock_WaitsThenStarts is the core scenario:
// a dependent service with DependsOn set must not get a container until
// its dependency has one running, and must proceed normally once it
// does, on the very next reconcile pass (level-triggered, no memory of
// the earlier blocked pass needed).
func TestController_DependencyBlock_WaitsThenStarts(t *testing.T) {
	backend := alwaysHealthy()
	defer backend.Close()

	rt := newFakeRuntime(serverPort(t, backend))
	desired := &store.DesiredService{
		Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80,
		DependsOn: []string{"db"},
	}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil while waiting on a dependency", err)
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "WaitingForDependency" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=WaitingForDependency", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: the dependent must not start before its dependency does", rt.createCalls)
	}

	// The dependency "myapp-db" now has a running container: reconciling
	// the dependent again (no state remembered from the blocked pass)
	// must now proceed.
	rt.seed(replicaContainerName("myapp-db", "postgres:16", "", 0), true)

	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil once the dependency has started", err)
	}
	cond = conditionOf(t, res)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True once the dependency has started", cond)
	}
	if rt.createCalls == 0 {
		t.Error("createCalls = 0, want at least 1: the dependent should have started its own container")
	}
}

// TestController_DependencyBlock_StoppedDependencyStillBlocks confirms a
// dependency container that exists but isn't running (created, crashed,
// or stopped) does not count as started: Running, not mere existence, is
// the signal.
func TestController_DependencyBlock_StoppedDependencyStillBlocks(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.seed(replicaContainerName("myapp-db", "postgres:16", "", 0), false)
	desired := &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "WaitingForDependency" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=WaitingForDependency for a non-running dependency container", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

// TestController_DependencyBlock_NoAppIDFailsOpen covers the brief
// window between DeploySpec's Deploy call and its own follow-up
// UpdateServiceApp (internal/deploy/multi.go): AppID isn't linked yet,
// so there is no way to resolve a bare dependency key into a real
// sibling name. The gate must not block forever in that window.
func TestController_DependencyBlock_NoAppIDFailsOpen(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if cond := conditionOf(t, res); cond.Reason == "WaitingForDependency" {
		t.Errorf("condition = %+v, want the gate skipped (fail open) when AppID is unset", cond)
	}
}

// listErrRuntime wraps fakeRuntime to make every ListByPrefix call fail,
// for the "dependency check itself failed" path dependencyBlock must
// surface as NotReady rather than silently proceeding or panicking.
type listErrRuntime struct {
	*fakeRuntime
	err error
}

func (r *listErrRuntime) ListByPrefix(_ context.Context, _ string) ([]docker.ContainerState, error) {
	return nil, r.err
}

func TestController_DependencyBlock_CheckFailure(t *testing.T) {
	rt := &listErrRuntime{fakeRuntime: newFakeRuntime(0), err: errors.New("docker daemon unavailable")}
	desired := &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: the failure surfaces as a condition, matching gpuPlacementBlock's own convention", err)
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "DependencyCheckFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=DependencyCheckFailed", cond)
	}
}

// createErrForPrefixRuntime wraps fakeRuntime so only container names
// with a given prefix fail to create, everything else behaves normally:
// unlike fakeRuntime's own createErr/createErrOnCall (global or call-
// order dependent), this fails a specific *service*'s container
// regardless of how many other services' Reconcile calls interleave
// with it, matching how a compose app's independently-reconciled
// sibling services share one real Docker daemon.
type createErrForPrefixRuntime struct {
	*fakeRuntime
	prefix string
	err    error
}

func (r *createErrForPrefixRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	if strings.HasPrefix(spec.Name, r.prefix) {
		return "", r.err
	}
	return r.fakeRuntime.Create(ctx, spec)
}

// TestController_DependencyBlock_HalfSucceededComposeApp is the
// half-succeeded scenario this feature's own PR description promises
// coverage for: a compose app's "db" service never starts (its image
// pull is broken), "web" depends_on db and must stay blocked, and
// "worker" (no dependency on db) must converge normally regardless,
// exactly the "one broken resource must not block convergence of
// everything else" principle the reconciler's own package doc comment
// states, with the one deliberate exception being a service that
// explicitly named the broken one as its own dependency.
func TestController_DependencyBlock_HalfSucceededComposeApp(t *testing.T) {
	backend := alwaysHealthy()
	defer backend.Close()

	rt := &createErrForPrefixRuntime{
		fakeRuntime: newFakeRuntime(serverPort(t, backend)),
		prefix:      "myapp-db-",
		err:         errors.New("pull access denied"),
	}

	db := New("myapp-db", &fakeStore{svc: &store.DesiredService{Name: "myapp-db", AppID: "myapp", Image: "postgres:16", Port: 5432}}, rt)
	web := New("myapp-web", &fakeStore{svc: &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}}, rt)
	worker := New("myapp-worker", &fakeStore{svc: &store.DesiredService{Name: "myapp-worker", AppID: "myapp", Image: "img:v1", Port: 80}}, rt)

	dbRes, dbErr := db.Reconcile(context.Background())
	if dbErr == nil {
		t.Fatal("db Reconcile() error = nil, want the injected create failure")
	}
	if cond := conditionOf(t, dbRes); cond.Status != reconcile.ConditionFalse {
		t.Errorf("db condition = %+v, want Status=False", cond)
	}

	webRes, webErr := web.Reconcile(context.Background())
	if webErr != nil {
		t.Fatalf("web Reconcile() error = %v, want nil (blocked, not failed)", webErr)
	}
	if cond := conditionOf(t, webRes); cond.Reason != "WaitingForDependency" {
		t.Errorf("web condition = %+v, want Reason=WaitingForDependency", cond)
	}

	workerRes, workerErr := worker.Reconcile(context.Background())
	if workerErr != nil {
		t.Fatalf("worker Reconcile() error = %v, want nil: an unrelated sibling must converge despite db's failure", workerErr)
	}
	if cond := conditionOf(t, workerRes); cond.Status != reconcile.ConditionTrue {
		t.Errorf("worker condition = %+v, want Status=True: an unrelated sibling must not be blocked by db's failure", cond)
	}
	if rt.createCalls == 0 {
		t.Error("worker never got a create call, want at least 1")
	}
}

// TestController_DependencyBlock_NoDependsOnUnaffected confirms an
// ordinary service with no DependsOn declared reconciles exactly as
// before this gate existed: no extra ListByPrefix call, no behavior
// change.
func TestController_DependencyBlock_NoDependsOnUnaffected(t *testing.T) {
	backend := alwaysHealthy()
	defer backend.Close()

	rt := newFakeRuntime(serverPort(t, backend))
	desired := &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True: a service with no DependsOn must be unaffected by this gate", cond)
	}
}
