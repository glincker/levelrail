package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
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
	apps       AppLister
	runtime    docker.Runtime
	prefix     string
	instanceID string
}

// NewNetworkCleanupController builds a NetworkCleanupController. prefix
// is the same value passed to every Controller's WithNetworkPrefix
// (typically brand.Brand.ShortName): both must agree, or this will
// never find the networks the per-service controllers actually create.
// instanceID is store.GetOrCreateInstanceID's result, the same value
// passed to every Controller's WithInstanceID; empty disables the
// instance-ownership check below, treating every matching-name network
// as this instance's own, this package's behavior before cross-instance
// safety existed.
func NewNetworkCleanupController(apps AppLister, runtime docker.Runtime, prefix, instanceID string) *NetworkCleanupController {
	return &NetworkCleanupController{apps: apps, runtime: runtime, prefix: prefix, instanceID: instanceID}
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
	observed = c.ownNetworks(observed)
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

// ownNetworks filters observed down to networks this same control-plane
// instance created, the same "list broadly, filter narrowly at the call
// site" split ListNetworksByPrefix's own doc comment describes. No
// instanceID configured (the default) is a no-op: every observed network
// is treated as this instance's own, this controller's behavior before
// cross-instance safety existed.
//
// Deliberately stricter than application.Controller.ownsInstance's
// analogous container check: a network with no instance label at all is
// EXCLUDED here, not kept. That's the opposite of ownsInstance's
// "unlabeled = mine" fallback, and on purpose. ownsInstance's fallback is
// safe because a name collision there additionally requires an identical
// content-hashed container name (see staleContainers' own doc comment);
// a network's name is just "<prefix>-app-<appID>", and appID is
// frequently a short, human-chosen slug (e.g. "verify-app"), so two
// independent Levelrail instances sharing one Docker daemon (a real,
// observed dev/multi-instance-per-host setup, not a hypothetical) can
// trivially produce the exact same network name without either instance
// doing anything wrong. Treating that unlabeled name match as "mine"
// already caused a live incident: a freshly started, empty-DB instance
// repeatedly attempted RemoveNetwork against another, unrelated running
// instance's network purely because neither the network nor (at that
// point) anything else distinguished the two. It only survived because
// Docker refused the delete while the other instance's containers were
// still attached.
//
// The cost of this stricter rule is that a network created by a
// pre-instance-labeling build of this same instance (so genuinely this
// instance's own orphan, just never labeled) is no longer
// auto-collected: Docker's network API has no way to attach a label to
// an existing network after creation, so there is no safe way to
// migrate it into the labeled set short of deleting and recreating it,
// which risks disrupting whatever is still attached. Leaving a rare
// pre-upgrade orphan uncollected (operator can remove it by hand) is the
// correct tradeoff against silently deleting another instance's live
// network. Only a network explicitly labeled with this instance's own
// ID is ever a cleanup candidate.
func (c *NetworkCleanupController) ownNetworks(observed []docker.NetworkInfo) []docker.NetworkInfo {
	if c.instanceID == "" {
		return observed
	}
	out := make([]docker.NetworkInfo, 0, len(observed))
	for _, n := range observed {
		if id, labeled := n.Labels[appspec.InstanceLabelKey]; labeled && id == c.instanceID {
			out = append(out, n)
		}
	}
	return out
}
