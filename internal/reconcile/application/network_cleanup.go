package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AppLister is the narrow surface NetworkCleanupController needs from
// internal/store: every currently-desired app. *store.DB satisfies this
// structurally.
type AppLister interface {
	ListApps(ctx context.Context) ([]store.App, error)
}

// NetworkCleanupController reconciles the whole fleet's per-app Docker
// networks in a single pass, the same "all of them, not one resource at
// a time" shape internal/reconcile/ingress's own controller already
// establishes for an analogous whole-fleet concern (that package's own
// doc comment explains why: there's no per-network incremental update
// primitive to converge one at a time against). Every network this
// codebase creates is created by name (NetworkName's own doc comment)
// only when a service's Controller actually needs one
// (createAndStart); nothing ever proactively deletes one, so this
// controller is the only place that ever does: it diffs Docker's own
// observed networks under prefix against store.App rows that still
// exist, and removes any network whose app is gone.
//
// Deliberately store-App-driven, not service-count-driven: a
// zero-service App (every service under it deleted individually, not
// via DeleteApp) still keeps its network, since a later deploy reusing
// that same app name is expected to find it again. Only an App row's
// own absence, from store.DeleteApp (internal/api's handleDeleteApp
// deletes it once removing a service leaves the App with no other
// members) or its cascade, means the network is actually orphaned.
type NetworkCleanupController struct {
	apps    AppLister
	runtime docker.Runtime
	prefix  string
}

// NewNetworkCleanupController builds a NetworkCleanupController. prefix
// is the same value passed to every Controller's WithNetworkPrefix
// (typically brand.Brand.ShortName): both must agree, or this will
// never find the networks the per-service controllers actually create.
func NewNetworkCleanupController(apps AppLister, runtime docker.Runtime, prefix string) *NetworkCleanupController {
	return &NetworkCleanupController{apps: apps, runtime: runtime, prefix: prefix}
}

// Name implements reconcile.Controller.
func (c *NetworkCleanupController) Name() string { return "application/network-cleanup" }

// Reconcile implements reconcile.Controller.
func (c *NetworkCleanupController) Reconcile(ctx context.Context) (reconcile.Result, error) {
	prefix := c.prefix
	if prefix == "" {
		prefix = defaultNetworkPrefix
	}
	networkPrefix := prefix + "-app-"

	observed, err := c.runtime.ListNetworksByPrefix(ctx, networkPrefix)
	if err != nil {
		return notReady("ListNetworksFailed", err), fmt.Errorf("application/network-cleanup: list networks: %w", err)
	}
	if len(observed) == 0 {
		return ready("NothingToClean"), nil
	}

	apps, err := c.apps.ListApps(ctx)
	if err != nil {
		return notReady("ListAppsFailed", err), fmt.Errorf("application/network-cleanup: list apps: %w", err)
	}
	wanted := make(map[string]bool, len(apps))
	for _, a := range apps {
		wanted[NetworkName(prefix, a.ID)] = true
	}

	var firstErr error
	removed := 0
	for _, n := range observed {
		if wanted[n.Name] {
			continue
		}
		if err := c.runtime.RemoveNetwork(ctx, n.Name); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove orphaned network %q: %w", n.Name, err)
			}
			continue
		}
		removed++
	}
	if firstErr != nil {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionTrue,
			Reason: "OrphanedNetworkCleanupFailed", Message: firstErr.Error(),
		}}}, fmt.Errorf("application/network-cleanup: %w", firstErr)
	}
	if removed > 0 {
		return ready("OrphanedNetworksRemoved"), nil
	}
	return ready("Converged"), nil
}
