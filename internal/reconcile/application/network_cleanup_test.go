package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// runtimeWithListNetworksErr wraps fakeRuntime to simulate ListNetworksByPrefix failing.
type runtimeWithListNetworksErr struct {
	*fakeRuntime
	err error
}

func (r *runtimeWithListNetworksErr) ListNetworksByPrefix(_ context.Context, _ string) ([]docker.NetworkInfo, error) {
	return nil, r.err
}

func TestNetworkCleanupController_Reconcile_ListNetworksFailed(t *testing.T) {
	baseRt := newFakeRuntime(0)
	rt := &runtimeWithListNetworksErr{fakeRuntime: baseRt, err: errors.New("docker API error")}
	apps := &fakeAppLister{}
	c := NewNetworkCleanupController(apps, rt, "test", "")

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want error")
	}
	cond := conditionOf(t, result)
	if cond.Reason != "ListNetworksFailed" {
		t.Errorf("Result Reason = %q, want %q", cond.Reason, "ListNetworksFailed")
	}
}

func TestNetworkCleanupController_Reconcile_ListAppsFailed(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.networks["test-app-1"] = "net-1"

	apps := &fakeAppLister{err: errors.New("db error")}
	c := NewNetworkCleanupController(apps, rt, "test", "")

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want error")
	}
	cond := conditionOf(t, result)
	if cond.Reason != "ListAppsFailed" {
		t.Errorf("Result Reason = %q, want %q", cond.Reason, "ListAppsFailed")
	}
}

func TestNetworkCleanupController_Reconcile_DefaultPrefix(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.networks[defaultNetworkPrefix+"-app-missing"] = "net-1"

	apps := &fakeAppLister{}
	c := NewNetworkCleanupController(apps, rt, "", "")

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Reason != "OrphanedNetworksRemoved" {
		t.Errorf("Result Reason = %q, want %q", cond.Reason, "OrphanedNetworksRemoved")
	}
	if len(rt.removedNetworks) != 1 || rt.removedNetworks[0] != defaultNetworkPrefix+"-app-missing" {
		t.Errorf("removed networks = %v, want [%q]", rt.removedNetworks, defaultNetworkPrefix+"-app-missing")
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
	c := NewNetworkCleanupController(apps, rt, "acme", "")

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
		t.Error("acme-app-long-gone was kept, want it removed (app no longer exists)")
	}
	if _, ok := rt.networks["unrelated-network"]; !ok {
		t.Error("unrelated-network was removed, want it untouched (doesn't match this prefix)")
	}
}

// TestNetworkCleanupController_DoesNotRemoveOtherInstanceNetworks proves
// the same cross-instance-safety property TestController_Teardown_
// DoesNotRemoveOtherInstanceContainers proves for containers, but for
// this controller's own whole-fleet network sweep: instance A's
// NetworkCleanupController must never remove a network another
// control-plane instance created and still owns, including one that
// carries no instance label at all. Unlike the analogous container path,
// an unlabeled network is never treated as "mine" here: unlike a
// container name, a network name ("<prefix>-app-<appID>") can collide
// across independent instances on the same Docker daemon without either
// instance doing anything wrong (this is what caused a real incident),
// so the only safe cleanup candidate is one explicitly labeled with this
// instance's own ID.
func TestNetworkCleanupController_DoesNotRemoveOtherInstanceNetworks(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.networks["acme-app-mine-and-gone"] = "net-1"
	rt.networkLabels["acme-app-mine-and-gone"] = map[string]string{"platform-reserved.instance": "inst-a"}
	rt.networks["acme-app-not-mine"] = "net-2"
	rt.networkLabels["acme-app-not-mine"] = map[string]string{"platform-reserved.instance": "inst-b"}
	rt.networks["acme-app-unlabeled-not-mine"] = "net-3" // no label at all: a different instance's network, or a pre-upgrade leftover of unknown origin

	apps := &fakeAppLister{} // instance A's own store has no apps at all
	c := NewNetworkCleanupController(apps, rt, "acme", "inst-a")

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "OrphanedNetworksRemoved" {
		t.Errorf("condition = %+v, want Status=True Reason=OrphanedNetworksRemoved", cond)
	}

	if _, ok := rt.networks["acme-app-mine-and-gone"]; ok {
		t.Error("acme-app-mine-and-gone was kept, want it removed (mine and orphaned)")
	}
	if _, ok := rt.networks["acme-app-not-mine"]; !ok {
		t.Error("acme-app-not-mine was removed, want it untouched (belongs to inst-b)")
	}
	if _, ok := rt.networks["acme-app-unlabeled-not-mine"]; !ok {
		t.Error("acme-app-unlabeled-not-mine was removed, want it untouched (no label = never a cleanup candidate, could belong to any instance)")
	}
}

// TestNetworkCleanupController_NothingToClean covers the common
// steady-state pass: no networks under this prefix exist yet, so this
// must be a cheap no-op, not an error.
func TestNetworkCleanupController_NothingToClean(t *testing.T) {
	rt := newFakeRuntime(0)
	apps := &fakeAppLister{}
	c := NewNetworkCleanupController(apps, rt, "acme", "")

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
	c := NewNetworkCleanupController(apps, rt, "acme", "")

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
