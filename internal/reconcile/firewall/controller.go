// Package firewall implements the reconcile.Controller that keeps the
// control plane's own host ufw rules (internal/firewall) in sync with
// every app service's HostPort pin and every managed database's public
// access port. Modeled directly on internal/reconcile/ingress: like
// ingress, this reconciles every exposed port in one pass rather than
// per-resource, since a single ufw invocation lists (and this
// controller diffs against) every rule at once, not one resource's own
// slice of it.
//
// v1 scope, stated plainly rather than silently assumed: this only
// manages ports for services/databases assigned to the control plane's
// own local node (NodeID == "", the same sentinel convention
// cmd/levelrail's resolveNodeTransport already uses). A resource pinned
// to a different, remote node is skipped with an informational log line
// and counted in the Result's message, not silently ignored: opening a
// port on a remote node would need a new agent RPC (the agent's gRPC
// transport has no generic exec/command call today, only Docker
// lifecycle operations), and rushing that protocol change to fit this
// feature isn't worth it. A future phase-3 follow-up can extend this
// controller to dispatch per-node once that RPC exists.
package firewall

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/firewall"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// localNodeID is the sentinel meaning "the control plane's own node",
// the same convention store.DesiredService.NodeID/DesiredDatabase.NodeID
// already document and cmd/levelrail's resolveNodeTransport already
// enforces elsewhere.
const localNodeID = ""

// ServiceStore is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database.
// *store.DB satisfies this.
type ServiceStore interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
}

// Syncer is the narrow surface this controller needs from
// internal/firewall.Manager, so tests can fake it without shelling out
// to a real ufw binary. *firewall.Manager satisfies this.
type Syncer interface {
	Sync(ctx context.Context, want []firewall.Rule) (firewall.Result, error)
}

// Controller converges the local host's ufw rules to match every
// service's HostPort and every database's public access port, for
// resources assigned to the control plane's own local node.
type Controller struct {
	store  ServiceStore
	syncer Syncer
	logger *slog.Logger
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithLogger overrides the logger used for per-resource skip decisions.
// Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *Controller) { c.logger = logger }
}

// New builds a Controller.
func New(svcStore ServiceStore, syncer Syncer, opts ...Option) *Controller {
	c := &Controller{store: svcStore, syncer: syncer, logger: slog.Default()}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "firewall" }

// Reconcile implements reconcile.Controller. It lists every desired
// service and database, builds the set of host ports that should be
// open for whichever of them are assigned to this controller's local
// node, and syncs that set against ufw via Syncer.Sync. A resource
// pinned to a remote node is skipped and logged, not treated as a
// reconcile failure: see the package doc comment for why that's this
// controller's stated v1 scope, not an oversight.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	services, err := c.store.ListDesiredServices(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("firewall: list desired services: %w", err)
	}
	databases, err := c.store.ListDesiredDatabases(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("firewall: list desired databases: %w", err)
	}

	want, skippedRemote := WantedRules(ctx, c.logger, services, databases)

	result, err := c.syncer.Sync(ctx, want)
	if err != nil {
		return notReady("SyncFailed", err), fmt.Errorf("firewall: sync: %w", err)
	}

	if !result.Installed {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "UFWNotInstalled",
			Message: "ufw is not installed; if you rely on a different firewall (cloud security groups, firewalld), this is expected and no ports are managed",
		}}}, nil
	}
	if !result.Active {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "UFWInactive",
			Message: "ufw is installed but inactive; no ports are managed until it's enabled",
		}}}, nil
	}
	if len(result.Errors) > 0 {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionFalse, Reason: "RuleSyncFailed",
			Message: fmt.Sprintf("%d of %d managed rule(s) failed to sync: %v", len(result.Errors), len(want), result.Errors),
		}}}, nil
	}

	reason := fmt.Sprintf("Synced%dRules", len(want))
	msg := fmt.Sprintf("%d rule(s) synced (%d opened, %d removed this pass)", len(want), result.Applied, result.Removed)
	if skippedRemote > 0 {
		msg += fmt.Sprintf(", %d resource(s) on a remote node skipped (not yet supported)", skippedRemote)
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionTrue, Reason: reason, Message: msg,
	}}}, nil
}

// WantedRules derives the set of firewall.Rule this platform wants open
// for services/databases assigned to the local node (NodeID ==
// localNodeID), from a single already-fetched list of each. Exported so
// internal/api's read-only firewall status endpoint can report the same
// desired set this controller reconciles toward, without a second,
// possibly-diverging copy of this filtering logic. skippedRemote counts
// resources excluded because they're pinned to a different node; see
// the package doc comment for why that's this package's stated v1
// scope, not silently dropped.
func WantedRules(ctx context.Context, logger *slog.Logger, services []store.DesiredService, databases []store.DesiredDatabase) (want []firewall.Rule, skippedRemote int) {
	for _, svc := range services {
		if svc.HostPort == nil {
			continue
		}
		if svc.NodeID != localNodeID {
			skippedRemote++
			logger.DebugContext(ctx, "firewall: service has a host port pinned to a remote node, skipping until per-node firewall management exists",
				slog.String("service", svc.Name), slog.String("node_id", svc.NodeID))
			continue
		}
		want = append(want, firewall.Rule{Port: *svc.HostPort, Owner: "app:" + svc.Name})
	}
	for _, db := range databases {
		if !db.PubliclyAccessible || db.PublicPort == 0 {
			continue
		}
		if db.NodeID != localNodeID {
			skippedRemote++
			logger.DebugContext(ctx, "firewall: database has a public port pinned to a remote node, skipping until per-node firewall management exists",
				slog.String("database", db.Name), slog.String("node_id", db.NodeID))
			continue
		}
		want = append(want, firewall.Rule{Port: db.PublicPort, Owner: "db:" + db.Name})
	}
	return want, skippedRemote
}

func notReady(reason string, err error) reconcile.Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}
