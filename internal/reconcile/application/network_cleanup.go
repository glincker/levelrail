package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
	nodeID     string
	grace      time.Duration
	maxPerPass int
	logger     *slog.Logger
	now        func() time.Time
}

// ForNode names this controller after a remote node so one runs per node
// without sharing a reconcile status.
func (c *NetworkCleanupController) ForNode(nodeID string) *NetworkCleanupController {
	c.nodeID = nodeID
	return c
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
	return &NetworkCleanupController{apps: apps, runtime: runtime, prefix: prefix, instanceID: instanceID, grace: NetworkGraceFromEnv(),
		maxPerPass: NetworkGCMaxPerPassFromEnv(), logger: slog.Default(), now: time.Now}
}

// Name implements reconcile.Controller.
func (c *NetworkCleanupController) Name() string {
	if c.nodeID != "" {
		return "application/network-cleanup/" + c.nodeID
	}
	return "application/network-cleanup"
}

// Reconcile implements reconcile.Controller.
func (c *NetworkCleanupController) Reconcile(ctx context.Context) (reconcile.Result, error) {
	prefix := c.prefix
	if prefix == "" {
		prefix = defaultNetworkPrefix
	}
	networkPrefix := prefix + "-app-"

	if admin, ok := c.runtime.(docker.NetworkAdmin); ok {
		return c.reconcileWithAdmin(ctx, admin, prefix)
	}
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
	observed = c.ownNetworks(observed)
	if len(observed) == 0 {
		return ready("NothingToClean"), nil
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
		if removed >= c.maxPerPass {
			c.logger.Info("application: network GC pass limit reached", slog.String("node_id", c.nodeID), slog.Int("limit", c.maxPerPass))
			break
		}
		if err := c.runtime.RemoveNetwork(ctx, n.Name); err != nil {
			if docker.IsNetworkInUse(err) {
				c.logger.Warn("application: orphaned network kept, unmanaged container attached", slog.String("node_id", c.nodeID), slog.String("network", n.Name), slog.String("network_id", n.ID))
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("remove orphaned network %q: %w", n.Name, err)
			}
			continue
		}
		removed++
		c.logger.Info("application: removed orphaned network", slog.String("node_id", c.nodeID), slog.String("network", n.Name), slog.String("network_id", n.ID))
	}
	if firstErr != nil {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue,
			Reason: "OrphanedNetworkCleanupFailed", Message: firstErr.Error(),
		}}}, fmt.Errorf("application/network-cleanup: %w", firstErr)
	}
	if removed > 0 {
		return ready("OrphanedNetworksRemoved"), nil
	}
	return ready("Converged"), nil
}

// ownNetworks keeps only networks labeled with this instance: an unlabeled name
// match can belong to a sibling instance on a shared daemon, so it is never collected.
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

// reconcileWithAdmin is the grace-period, never-force GC used when the
// runtime can report attached containers (a local Docker client).
func (c *NetworkCleanupController) reconcileWithAdmin(ctx context.Context, admin docker.NetworkAdmin, prefix string) (reconcile.Result, error) {
	apps, err := c.apps.ListApps(ctx)
	if err != nil {
		return notReady("ListAppsFailed", err), fmt.Errorf("application/network-cleanup: list apps: %w", err)
	}
	details, err := c.describeNetworks(ctx, admin, prefix, apps)
	if err != nil {
		return notReady("ListNetworksFailed", err), fmt.Errorf("application/network-cleanup: list networks: %w", err)
	}
	if len(details) == 0 {
		return ready("NothingToClean"), nil
	}
	var firstErr error
	removed := 0
	for _, v := range ClassifyNetworks(prefix, c.instanceID, details, apps, c.now(), c.grace) {
		if !v.Removable {
			continue
		}
		if removed >= c.maxPerPass {
			c.logger.Info("application: network GC pass limit reached", slog.String("node_id", c.nodeID), slog.Int("limit", c.maxPerPass))
			break
		}
		ok, err := admin.RemoveNetworkIfEmpty(ctx, v.Name)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove orphaned network %q: %w", v.Name, err)
			}
			continue
		}
		if ok {
			removed++
			c.logger.Info("application: removed orphaned network", slog.String("node_id", c.nodeID), slog.String("network", v.Name), slog.String("network_id", v.ID))
		}
	}
	if firstErr != nil {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue,
			Reason: "OrphanedNetworkCleanupFailed", Message: firstErr.Error(),
		}}}, fmt.Errorf("application/network-cleanup: %w", firstErr)
	}
	if removed > 0 {
		return ready("OrphanedNetworksRemoved"), nil
	}
	return ready("Converged"), nil
}

// describeNetworks inspects only orphans of this instance: ClassifyNetworks reads
// the attached count solely to decide whether such a network is safe to remove.
func (c *NetworkCleanupController) describeNetworks(ctx context.Context, admin docker.NetworkAdmin, prefix string, apps []store.App) ([]docker.NetworkDetail, error) {
	sel, ok := admin.(docker.SelectiveNetworkDescriber)
	if !ok {
		return admin.DescribeNetworks(ctx, prefix+"-app-")
	}
	wanted := make(map[string]bool, len(apps))
	for _, a := range apps {
		wanted[NetworkName(prefix, a.ID)] = true
	}
	return sel.DescribeNetworksSelective(ctx, prefix+"-app-", func(name string, labels map[string]string) bool {
		if wanted[name] {
			return false
		}
		owner, labeled := labels[appspec.InstanceLabelKey]
		return c.instanceID == "" || !labeled || owner == c.instanceID
	})
}
