// Package mesh implements TASKS.md 3.4's mesh controller: the
// reconcile.Controller that keeps the WireGuard mesh and the internal DNS
// zone converged on whatever the store currently says the fleet and its
// placements look like.
//
// Like internal/reconcile/ingress, and unlike the per-resource
// application and database controllers, this reconciles the whole fleet
// in one pass. That is not a convenience: a mesh is a property of the set
// of nodes, not of any one node, and every node's peer list changes when
// any node's key or address changes. There is no coherent
// "reconcile one node's mesh" operation to build.
//
// Each pass:
//
//  1. Reads every node and every placement from the store. Nothing about
//     the fleet is cached between passes (CLAUDE.md 4.2).
//  2. Distributes mesh configuration through internal/network.Coordinator,
//     which allocates any missing addresses, plans a full mesh, and sends
//     each node its own config.
//  3. Persists whatever the nodes reported back (their public keys) plus
//     any newly allocated address, so the next pass plans from it.
//  4. Rebuilds the internal DNS record set from the same node data and
//     the current placements, and swaps it into the resolver.
//
// Ordering matters between 3 and 4 and only in one direction: DNS records
// are built from the same in-memory inventory the distribution pass just
// updated, not from a re-read of the store, so a node whose address was
// allocated this very pass resolves immediately rather than one pass
// later.
//
// What this controller does not do, and why:
//
//   - It does not create or own the Mesh device. That belongs to whoever
//     builds the process (the control plane's own node) or to the agent
//     (a remote node), because a Mesh is a live handle on a kernel
//     interface with a lifetime tied to the process, not to a reconcile
//     pass.
//   - It does not point containers at the DNS server. See
//     internal/network's dns_server.go for exactly what that needs and
//     why it is a separate change.
package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the narrow surface this controller needs from internal/store,
// so tests can fake it without a real database. *store.DB satisfies it.
type Store interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	UpdateNodeMesh(ctx context.Context, id, publicKey, address string) error
}

// Distributor is the narrow surface this controller needs from
// internal/network.Coordinator, so tests can drive the controller without
// building a coordinator and a sink. *network.Coordinator satisfies it.
type Distributor interface {
	Distribute(ctx context.Context, nodes []network.NodeInfo) ([]network.NodeInfo, network.DistributeResult, error)
}

// Controller reconciles the mesh and the internal DNS zone.
type Controller struct {
	// selfID is the control plane's own node ID, needed because
	// desired_services.node_id uses the empty string to mean "this node"
	// (migrations/0009_node_placement.sql) and DNS records need a real
	// node to resolve that to.
	selfID string

	store       Store
	distributor Distributor
	resolver    *network.Resolver
	logger      *slog.Logger
}

// Option configures New.
type Option func(*Controller)

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Controller) {
		if l != nil {
			c.logger = l
		}
	}
}

// New builds the mesh controller. resolver is swapped wholesale on every
// pass and may be shared with a running network.DNSServer: that is the
// intended wiring, since network.Resolver.SetRecords is safe for
// concurrent use precisely so a reconcile pass can update it while
// queries are in flight.
func New(selfID string, st Store, distributor Distributor, resolver *network.Resolver, opts ...Option) *Controller {
	c := &Controller{
		selfID:      selfID,
		store:       st,
		distributor: distributor,
		resolver:    resolver,
		logger:      slog.Default(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name identifies this controller in logs and status lookups.
func (c *Controller) Name() string { return "mesh" }

// Reconcile runs one full pass. See the package doc comment for the
// order and why.
//
// Returns conditions even when it also returns an error, per
// reconcile.Controller's contract. Three conditions, not one, because
// three genuinely different things can be wrong and collapsing them
// would make the UI say "mesh degraded" without saying which half:
//
//   - MeshPlanned: the inventory was coherent enough to plan from. False
//     here means a duplicate key or an exhausted address range, and
//     nothing was distributed at all.
//   - MeshDistributed: every node accepted its config. False means at
//     least one node is unreachable, which is expected and transient
//     while the rest of the fleet still converged.
//   - DNSResolvable: every placement resolved to a real mesh address.
//     False means some service name will answer NXDOMAIN, which is a
//     broken connection string, not a cosmetic gap.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	nodes, err := c.store.ListNodes(ctx)
	if err != nil {
		return failed("MeshPlanned", "StoreUnavailable", err.Error()), fmt.Errorf("mesh: list nodes: %w", err)
	}

	inventory, invalid := c.toInventory(nodes)
	if len(invalid) > 0 {
		// A node whose stored mesh state does not parse is dropped from
		// the inventory rather than failing the pass, the same
		// "one broken resource must never block the rest" handling
		// dynamicSource applies to an unreachable node. It is logged and
		// surfaced, never silent: a node that cannot be planned for is a
		// node nothing can reach.
		for _, bad := range invalid {
			c.logger.Warn("node has unusable mesh state, excluding it from this pass",
				slog.String("node_id", bad.nodeID), slog.String("error", bad.reason))
		}
	}

	updated, result, err := c.distributor.Distribute(ctx, inventory)
	if err != nil {
		return failed("MeshPlanned", "InventoryInvalid", err.Error()), fmt.Errorf("mesh: distribute: %w", err)
	}

	conditions := []reconcile.Condition{
		condition("MeshPlanned", reconcile.ConditionTrue, "Planned",
			fmt.Sprintf("planned a full mesh across %d nodes", len(updated))),
	}
	conditions = append(conditions, c.persist(ctx, updated, result))
	conditions = append(conditions, c.refreshDNS(ctx, updated))

	return reconcile.Result{Conditions: conditions}, nil
}

// persist writes back what the nodes reported and reports whether every
// node accepted its config.
func (c *Controller) persist(ctx context.Context, inventory []network.NodeInfo, result network.DistributeResult) reconcile.Condition {
	byID := make(map[string]network.NodeInfo, len(inventory))
	for _, n := range inventory {
		byID[n.ID] = n
	}

	var persistErrs int
	for _, id := range result.Updated {
		n, known := byID[id]
		if !known {
			continue
		}
		addr := ""
		if n.Address.IsValid() {
			addr = n.Address.String()
		}
		if err := c.store.UpdateNodeMesh(ctx, id, n.PublicKey.String(), addr); err != nil {
			// Not fatal to the pass: the next one recomputes and retries,
			// and the mesh itself is already configured. What is lost is
			// only the persistence, which means one extra pass to
			// converge, not a broken mesh.
			persistErrs++
			c.logger.Error("persisting mesh state failed",
				slog.String("node_id", id), slog.String("error", err.Error()))
		}
	}

	// An address allocated this pass for a node that reported nothing new
	// still has to be persisted, or it is reallocated from scratch next
	// pass and the node is renumbered.
	for _, n := range inventory {
		if !n.Address.IsValid() || contains(result.Updated, n.ID) {
			continue
		}
		if err := c.store.UpdateNodeMesh(ctx, n.ID, n.PublicKey.String(), n.Address.String()); err != nil {
			persistErrs++
			c.logger.Error("persisting mesh address failed",
				slog.String("node_id", n.ID), slog.String("error", err.Error()))
		}
	}

	failedNodes := result.Failed()
	switch {
	case len(failedNodes) > 0:
		return condition("MeshDistributed", reconcile.ConditionFalse, "NodeUnreachable",
			fmt.Sprintf("%d of %d nodes did not accept their mesh config this pass", len(failedNodes), len(result.Nodes)))
	case persistErrs > 0:
		return condition("MeshDistributed", reconcile.ConditionFalse, "PersistFailed",
			fmt.Sprintf("every node was configured, but %d mesh state writes failed", persistErrs))
	default:
		return condition("MeshDistributed", reconcile.ConditionTrue, "Distributed",
			fmt.Sprintf("every one of %d nodes accepted its mesh config", len(result.Nodes)))
	}
}

// refreshDNS rebuilds the internal DNS zone from the current placements
// and the inventory the distribution pass just updated.
func (c *Controller) refreshDNS(ctx context.Context, inventory []network.NodeInfo) reconcile.Condition {
	placements, err := c.placements(ctx)
	if err != nil {
		return condition("DNSResolvable", reconcile.ConditionUnknown, "StoreUnavailable", err.Error())
	}

	set, err := network.BuildRecords(c.resolver.Zone(), c.selfID, placements, inventory)
	if err != nil {
		// A duplicate service name, the one BuildRecords failure that is
		// not recoverable. The previous record set stays in place rather
		// than being replaced with a broken one: stale-but-working beats
		// correct-and-empty when the alternative is every internal name
		// answering NXDOMAIN.
		return condition("DNSResolvable", reconcile.ConditionFalse, "RecordsInvalid", err.Error())
	}

	c.resolver.SetRecords(set)

	if len(set.Unresolved) > 0 {
		for _, u := range set.Unresolved {
			c.logger.Warn("service has no resolvable internal DNS name",
				slog.String("service", u.Service), slog.String("node_id", u.NodeID),
				slog.String("reason", u.Reason))
		}
		return condition("DNSResolvable", reconcile.ConditionFalse, "PlacementUnresolvable",
			fmt.Sprintf("%d of %d services have no resolvable address", len(set.Unresolved), len(set.Unresolved)+len(set.Records)))
	}
	return condition("DNSResolvable", reconcile.ConditionTrue, "Resolvable",
		fmt.Sprintf("%d internal names resolve in zone %s", len(set.Records), set.Zone))
}

// placements collects every service and database the store knows about.
// Databases matter at least as much as services here: the exit criterion
// this whole subsystem exists for is a database connection string
// surviving a node move.
func (c *Controller) placements(ctx context.Context) ([]network.Placement, error) {
	services, err := c.store.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("mesh: list desired services: %w", err)
	}
	databases, err := c.store.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("mesh: list desired databases: %w", err)
	}

	out := make([]network.Placement, 0, len(services)+len(databases))
	for _, s := range services {
		out = append(out, network.Placement{Service: s.Name, NodeID: s.NodeID})
	}
	for _, d := range databases {
		out = append(out, network.Placement{Service: d.Name, NodeID: d.NodeID})
	}
	return out, nil
}

// badNode is one store row that could not be turned into a NodeInfo.
type badNode struct {
	nodeID string
	reason string
}

// toInventory converts store rows into the value type internal/network
// plans over, parsing and validating the two string columns 0010 stores.
// The conversion happens here rather than in internal/store precisely so
// the mesh's own rules live in one place; see store.Node's own comment.
func (c *Controller) toInventory(nodes []store.Node) ([]network.NodeInfo, []badNode) {
	inventory := make([]network.NodeInfo, 0, len(nodes))
	var invalid []badNode

	for _, n := range nodes {
		info := network.NodeInfo{ID: n.ID, Name: n.Name, Endpoint: n.Address}

		if n.MeshPublicKey != "" {
			key, err := network.ParseKey(n.MeshPublicKey)
			if err != nil {
				invalid = append(invalid, badNode{nodeID: n.ID, reason: err.Error()})
				continue
			}
			info.PublicKey = key
		}
		if n.MeshAddress != "" {
			addr, err := netip.ParseAddr(n.MeshAddress)
			if err != nil {
				invalid = append(invalid, badNode{nodeID: n.ID, reason: fmt.Sprintf("mesh address %q: %v", n.MeshAddress, err)})
				continue
			}
			info.Address = addr
		}
		// A stored endpoint that does not parse is dropped rather than
		// disqualifying the node: WireGuard learns a peer's endpoint from
		// its first inbound handshake anyway (ADR 003's reverse-dial
		// design guarantees the node initiates), so a missing endpoint
		// costs one handshake round, where excluding the node entirely
		// would cost its whole presence on the mesh.
		if endpoint, err := network.ParseEndpoint(n.Address); err == nil {
			info.Endpoint = endpoint
		} else {
			info.Endpoint = ""
			c.logger.Warn("node has an unusable address, leaving its endpoint unset",
				slog.String("node_id", n.ID), slog.String("error", err.Error()))
		}

		inventory = append(inventory, info)
	}
	return inventory, invalid
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func condition(kind string, status reconcile.ConditionStatus, reason, message string) reconcile.Condition {
	return reconcile.Condition{
		Type:               kind,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: time.Now().UTC(),
	}
}

func failed(kind, reason, message string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{
		condition(kind, reconcile.ConditionFalse, reason, message),
	}}
}
