package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNetworkName(t *testing.T) {
	tests := []struct {
		name, prefix, appID, want string
	}{
		{"explicit prefix", "acme", "app_web", "acme-app-app_web"},
		{"empty prefix falls back to default", "", "app_web", "platform-app-app_web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NetworkName(tt.prefix, tt.appID); got != tt.want {
				t.Errorf("NetworkName(%q, %q) = %q, want %q", tt.prefix, tt.appID, got, tt.want)
			}
		})
	}
}

func TestServiceAlias(t *testing.T) {
	tests := []struct {
		name string
		svc  *store.DesiredService
		want string
	}{
		{
			name: "multi-service: strips appID- prefix",
			svc:  &store.DesiredService{Name: "myapp-web", AppID: "myapp"},
			want: "web",
		},
		{
			name: "single-service: name equals app, falls back to full name",
			svc:  &store.DesiredService{Name: "web", AppID: "web"},
			want: "web",
		},
		{
			name: "name shares appID as substring but not as a prefix boundary",
			svc:  &store.DesiredService{Name: "webworker", AppID: "web"},
			want: "webworker", // "webworker" has prefix "web-"? no: CutPrefix requires "web-" exactly
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceAlias(tt.svc); got != tt.want {
				t.Errorf("serviceAlias(%+v) = %q, want %q", tt.svc, got, tt.want)
			}
		})
	}
}

// TestController_Reconcile_AttachesToPerAppNetwork is Part A's core
// creation-path scenario: a service with a non-empty AppID gets its
// per-app network ensured before the container that needs it is
// created, and the container's own ContainerSpec.Network carries the
// right name and alias.
func TestController_Reconcile_AttachesToPerAppNetwork(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, AppID: "myapp"}
	c := New("myapp-web", &fakeStore{svc: desired}, rt, WithNetworkPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v, want Status=True Reason=Deployed", cond)
	}

	wantNetwork := "acme-app-myapp"
	if rt.ensureNetworkCalls != 1 {
		t.Errorf("ensureNetworkCalls = %d, want 1", rt.ensureNetworkCalls)
	}
	if _, ok := rt.networks[wantNetwork]; !ok {
		t.Errorf("networks = %+v, want %q created", rt.networks, wantNetwork)
	}
	if rt.lastCreateSpec.Network == nil {
		t.Fatal("lastCreateSpec.Network = nil, want a NetworkAttachment")
	}
	if rt.lastCreateSpec.Network.Name != wantNetwork {
		t.Errorf("Network.Name = %q, want %q", rt.lastCreateSpec.Network.Name, wantNetwork)
	}
	if rt.lastCreateSpec.Network.Alias != "web" {
		t.Errorf("Network.Alias = %q, want %q", rt.lastCreateSpec.Network.Alias, "web")
	}

	// Ordering: the network must exist before the container that needs
	// it is created, never the reverse. Exactly one of each in this
	// fresh-deploy scenario, so a strict two-element sequence is enough.
	if len(rt.callOrder) != 2 || rt.callOrder[0] != "network:"+wantNetwork {
		t.Fatalf("callOrder = %v, want [network:%s, create:...]", rt.callOrder, wantNetwork)
	}
	if len(rt.callOrder[1]) < 7 || rt.callOrder[1][:7] != "create:" {
		t.Errorf("callOrder[1] = %q, want a create: entry after the network", rt.callOrder[1])
	}
}

// TestController_Reconcile_SingleServiceApp_StillGetsNetwork proves item
// 3's requirement directly: a 1-service app (AppID set, e.g. via
// migrations/0039_apps.sql's own backfill convention where AppID equals
// the service's own name) is not special-cased away from networking.
func TestController_Reconcile_SingleServiceApp_StillGetsNetwork(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "web"}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.ensureNetworkCalls != 1 {
		t.Errorf("ensureNetworkCalls = %d, want 1 (a 1-service app must still get a network)", rt.ensureNetworkCalls)
	}
	if rt.lastCreateSpec.Network == nil || rt.lastCreateSpec.Network.Alias != "web" {
		t.Errorf("Network = %+v, want alias %q", rt.lastCreateSpec.Network, "web")
	}
}

// TestController_Reconcile_NoAppID_NoNetworkAttachment covers a service
// with no AppID at all (created before migrations/0039_apps.sql ever
// ran, and never linked since): Docker's plain default bridge network,
// unchanged, exactly like every container this codebase created before
// this field existed.
func TestController_Reconcile_NoAppID_NoNetworkAttachment(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.ensureNetworkCalls != 0 {
		t.Errorf("ensureNetworkCalls = %d, want 0", rt.ensureNetworkCalls)
	}
	if rt.lastCreateSpec.Network != nil {
		t.Errorf("Network = %+v, want nil", rt.lastCreateSpec.Network)
	}
}

// TestController_Reconcile_IdempotentReconcile_AlreadyNetworked proves
// re-running Reconcile against an already-running, already-networked
// container is a true no-op: no EnsureNetwork call, no Create call,
// nothing recreated. Network attachment only ever happens at container
// creation time (Docker has no "reattach an existing container" call
// this controller needs), so the level-triggered check that matters
// here is the same one every other idempotency test in this package
// already covers: does the right container exist and is it running.
func TestController_Reconcile_IdempotentReconcile_AlreadyNetworked(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("myapp-web", desired.Image, "")
	rt.seed(target, true)

	c := New("myapp-web", &fakeStore{svc: desired}, rt, WithNetworkPrefix("acme"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (already converged, must be a no-op)", rt.createCalls)
	}
	if rt.ensureNetworkCalls != 0 {
		t.Errorf("ensureNetworkCalls = %d, want 0 (nothing to (re)attach on an already-running container)", rt.ensureNetworkCalls)
	}

	// A second Reconcile pass changes nothing further either.
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if rt.createCalls != 0 || rt.ensureNetworkCalls != 0 {
		t.Errorf("after second Reconcile: createCalls=%d ensureNetworkCalls=%d, want 0/0", rt.createCalls, rt.ensureNetworkCalls)
	}
}

// TestController_Reconcile_EnsureNetworkFails_ContainerNeverCreated is
// the half-succeeded case CLAUDE.md's testing standard requires: a
// network operation that fails before the container operation ever
// runs, so nothing partially exists.
func TestController_Reconcile_EnsureNetworkFails_ContainerNeverCreated(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.ensureNetworkErr = errors.New("docker daemon unavailable")
	desired := &store.DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, AppID: "myapp"}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the network error to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a failed network ensure must block container creation, not half-create", rt.createCalls)
	}
}

// TestController_Reconcile_NetworkSucceeds_ContainerCreateFails is the
// half-succeeded case's other direction: the network now exists (a real
// side effect that happened), but the container itself failed to
// create. A retrying Reconcile must not error on or recreate the
// already-existing network; EnsureNetwork's own idempotency is what
// makes that safe.
func TestController_Reconcile_NetworkSucceeds_ContainerCreateFails(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.createErr = errors.New("engine rejected create")
	desired := &store.DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, AppID: "myapp"}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the create error to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=CreateFailed", cond)
	}
	if rt.ensureNetworkCalls != 1 {
		t.Errorf("ensureNetworkCalls = %d, want 1: the network step itself succeeded", rt.ensureNetworkCalls)
	}
	if len(rt.networks) != 1 {
		t.Fatalf("networks = %+v, want exactly one network left behind by the half-succeeded attempt", rt.networks)
	}

	// Retry: EnsureNetwork must be idempotent, not error or duplicate.
	rt.createErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("retry Reconcile() error = %v", err)
	}
	if rt.ensureNetworkCalls != 2 {
		t.Errorf("ensureNetworkCalls after retry = %d, want 2 (called again, idempotently)", rt.ensureNetworkCalls)
	}
	if len(rt.networks) != 1 {
		t.Errorf("networks after retry = %+v, want still exactly one (no duplicate created)", rt.networks)
	}
}

// TestNetworkCleanupController_RemovesOrphanedNetworks is the cleanup
// path's core scenario: a network whose app no longer exists in the
// store gets removed; a network whose app still exists does not.
func TestNetworkCleanupController_RemovesOrphanedNetworks(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.networks["acme-app-still-here"] = "net-1"
	rt.networks["acme-app-long-gone"] = "net-2"
	rt.networks["unrelated-network"] = "net-3" // no matching prefix, never touched

	apps := &fakeAppLister{apps: []store.App{{ID: "still-here", Name: "still-here"}}}
	c := NewNetworkCleanupController(apps, rt, "acme")

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "OrphanedNetworksRemoved" {
		t.Errorf("condition = %+v, want Status=True Reason=OrphanedNetworksRemoved", cond)
	}

	if _, ok := rt.networks["acme-app-still-here"]; !ok {
		t.Error("acme-app-still-here was removed, want it kept (app still exists)")
	}
	if _, ok := rt.networks["acme-app-long-gone"]; ok {
		t.Error("acme-app-long-gone still present, want it removed (app no longer exists)")
	}
	if _, ok := rt.networks["unrelated-network"]; !ok {
		t.Error("unrelated-network was removed, want it untouched (doesn't match this prefix)")
	}
}

// TestNetworkCleanupController_NothingToClean covers the common
// steady-state pass: no networks under this prefix exist yet, so this
// must be a cheap no-op, not an error.
func TestNetworkCleanupController_NothingToClean(t *testing.T) {
	rt := newFakeRuntime(0)
	apps := &fakeAppLister{}
	c := NewNetworkCleanupController(apps, rt, "acme")

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "NothingToClean" {
		t.Errorf("condition = %+v, want Status=True Reason=NothingToClean", cond)
	}
}

// TestNetworkCleanupController_RemoveFails_HalfSucceeded is the cleanup
// controller's own half-succeeded case: one orphaned network fails to
// remove, another succeeds; the failure surfaces without silently
// dropping the ones that did succeed.
func TestNetworkCleanupController_RemoveFails_HalfSucceeded(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.networks["acme-app-gone-1"] = "net-1"
	rt.networks["acme-app-gone-2"] = "net-2"
	rt.removeNetworkErr = errors.New("network has active endpoints")

	apps := &fakeAppLister{}
	c := NewNetworkCleanupController(apps, rt, "acme")

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the remove failure to propagate")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "OrphanedNetworkCleanupFailed" {
		t.Errorf("condition = %+v, want Status=True Reason=OrphanedNetworkCleanupFailed (the whole pass isn't fatal)", cond)
	}
}

// fakeAppLister is a hand-written fake for AppLister.
type fakeAppLister struct {
	apps []store.App
	err  error
}

func (f *fakeAppLister) ListApps(_ context.Context) ([]store.App, error) {
	return f.apps, f.err
}

var _ docker.Runtime = (*fakeRuntime)(nil)
