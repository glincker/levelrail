package network

// This file: TASKS.md 3.4's peer key distribution, control-plane side.
//
// The exchange, and why it is an exchange rather than a push:
//
// A node's private key must never leave that node. That is not a
// convention this code follows, it is a property the protocol shape makes
// impossible to violate: the control plane never generates a key, never
// has a field to put one in (DeviceConfig.PrivateKey is zero on
// everything that crosses the wire, see its doc comment), and never asks
// for one. What it asks for is what only the node can supply, which is
// its *public* key. Meanwhile the node cannot pick its own mesh address
// without risking a collision with another node picking concurrently, so
// the control plane supplies that.
//
// Each node therefore holds one half of its own configuration and the
// control plane holds the other, which means one pass cannot possibly
// complete the mesh for a brand new node: the control plane sends what it
// has (an address, and every peer it already knows), the node applies it
// and answers with its public key, and the *next* pass is the one where
// that key reaches the other nodes. That is not a wart to be engineered
// away with a bootstrap handshake; it is ordinary level-triggered
// convergence (CLAUDE.md 4.2), the same "it takes as many passes as it
// takes, and every pass is safe to interrupt" property every reconciler
// here already has. A node joining a fleet is fully meshed within two
// reconcile intervals.
//
// Scope, stated plainly: the ConfigSink interface below is the boundary,
// and the gRPC implementation of it (a new op on the agent Session
// stream) is deliberately not in this change. See ConfigSink's own doc
// comment for exactly what that implementation needs and why it is
// separated.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// NodeIdentity is what a node reports back about itself when it applies a
// mesh config: the half of its configuration only it can know.
type NodeIdentity struct {
	// PublicKey is derived from the private key the node generated and
	// keeps locally. This is the only key material that ever moves.
	PublicKey Key

	// ListenPort is the UDP port the node's device actually bound, which
	// can differ from the one it was asked for if that port was taken.
	ListenPort int

	// Endpoint is the node's own view of how it can be reached
	// ("host:port"), which may be empty. It is a hint, not an authority:
	// a node behind NAT genuinely does not know its own public address,
	// and the control plane's observation of where the node's gRPC
	// session dialed from is a better source. Coordinator prefers the
	// observed value when it has one; see Distribute.
	Endpoint string
}

// ConfigSink delivers one node's DeviceConfig to that node and returns
// what the node reports back.
//
// An interface, and the only thing this package knows about transport,
// because the real implementation belongs to the agent Session stream and
// that stream's wire contract (proto/agent/v1/agent.proto) is on
// CLAUDE.md section 8's do-not-parallelize list. Extending it is a small
// and entirely mechanical change: one new arm on AgentRequest.op and one
// on AgentResponse.result, carrying the fields of DeviceConfig (minus
// PrivateKey, which never crosses) and NodeIdentity respectively, plus a
// case in internal/agent.Execute that calls Mesh.Apply. It is left out of
// this change so that it lands as its own reviewed diff against the
// transport, rather than as a wire-contract change buried inside a
// networking one.
//
// What ships here instead: this interface, an in-process implementation
// (LocalSink) that is exactly what a single-node control plane needs and
// what the reconcile path exercises today, and the full distribution
// logic above it, all of it testable without a network.
type ConfigSink interface {
	// ApplyMesh sends cfg to nodeID and returns that node's reported
	// identity. An error means this one node did not get its config;
	// Distribute treats that as this node's problem, never the fleet's.
	ApplyMesh(ctx context.Context, nodeID string, cfg DeviceConfig) (NodeIdentity, error)
}

// NodeResult is the outcome of distributing to one node.
type NodeResult struct {
	NodeID   string
	Config   DeviceConfig
	Identity NodeIdentity

	// Err is non-nil when this node could not be reached or refused the
	// config. Recorded per node rather than aborting the pass: this is
	// the same principle dynamicSource already applies when a node's
	// transport is unavailable ("one broken resource must never block the
	// rest"), and it matters more here, because the node most likely to
	// be unreachable is the one that just went down, and that is exactly
	// when the *other* nodes most need an updated peer list.
	Err error
}

// DistributeResult is one full distribution pass.
type DistributeResult struct {
	// Nodes is one entry per node in the inventory, in node ID order.
	Nodes []NodeResult

	// Updated lists the nodes whose reported identity differed from what
	// the inventory held going in. A non-empty Updated means the caller
	// must persist those identities and that the next pass will produce a
	// different plan; an empty one means the mesh has converged.
	Updated []string
}

// Failed reports whether any node failed. Distribute itself returns a nil
// error in that case (see NodeResult.Err), so this is how a caller asks.
func (r DistributeResult) Failed() []NodeResult {
	var out []NodeResult
	for _, n := range r.Nodes {
		if n.Err != nil {
			out = append(out, n)
		}
	}
	return out
}

// Coordinator distributes mesh configuration across a fleet.
//
// Holds no state about the fleet between passes, on purpose: every
// Distribute call is given the complete inventory and recomputes
// everything from it. Caching "what I sent last time" would make this
// edge-triggered, and a control plane restart would then leave the fleet
// in whatever state it was in with nothing to reconcile it back.
type Coordinator struct {
	sink   ConfigSink
	opts   PlanOptions
	logger *slog.Logger

	// observedEndpoints maps node ID to the address the control plane
	// saw that node dial in from. Supplied by the caller
	// (SetObservedEndpoint), not discovered here: this package has no
	// access to the gRPC session, and should not.
	//
	// Guarded, because the two callers are genuinely concurrent and by
	// design: SetObservedEndpoint is called from whatever handles an
	// agent connecting or disconnecting, while Distribute runs on the
	// reconcile loop. This is the one piece of state a Coordinator keeps
	// between passes, which is exactly why it needs the lock and why
	// nothing else here does.
	mu                sync.RWMutex
	observedEndpoints map[string]string
}

// CoordinatorOption configures NewCoordinator.
type CoordinatorOption func(*Coordinator)

// WithCoordinatorLogger sets the structured logger.
func WithCoordinatorLogger(l *slog.Logger) CoordinatorOption {
	return func(c *Coordinator) {
		if l != nil {
			c.logger = l
		}
	}
}

// NewCoordinator builds a Coordinator that distributes through sink using
// opts. A zero PlanOptions.MeshCIDR is filled in with DefaultMeshCIDR
// rather than rejected, so a caller that has no opinion about addressing
// gets a working mesh instead of an error about a field they did not know
// existed.
func NewCoordinator(sink ConfigSink, opts PlanOptions, options ...CoordinatorOption) *Coordinator {
	if !opts.MeshCIDR.IsValid() {
		opts.MeshCIDR = DefaultMeshCIDR
	}
	c := &Coordinator{
		sink:              sink,
		opts:              opts,
		logger:            slog.Default(),
		observedEndpoints: map[string]string{},
	}
	for _, o := range options {
		o(c)
	}
	return c
}

// SetObservedEndpoint records where the control plane saw nodeID connect
// from. Passing "" forgets a previously observed endpoint, which is what
// should happen when a node disconnects: a stale endpoint for a node on a
// dynamic address is worse than none, since WireGuard will keep sending
// handshake initiations to whoever holds that address now.
func (c *Coordinator) SetObservedEndpoint(nodeID, endpoint string) error {
	if nodeID == "" {
		return fmt.Errorf("%w: cannot record an endpoint for an empty node ID", ErrUnknownNode)
	}
	if endpoint == "" {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.observedEndpoints, nodeID)
		return nil
	}
	normalized, err := ParseEndpoint(endpoint)
	if err != nil {
		return fmt.Errorf("record observed endpoint for node %q: %w", nodeID, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observedEndpoints[nodeID] = normalized
	return nil
}

// observedEndpoint reads one node's observed endpoint under the lock.
func (c *Coordinator) observedEndpoint(nodeID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.observedEndpoints[nodeID]
	return e, ok
}

// Distribute runs one full pass: allocate any missing addresses, plan
// every node's config, send each one, and collect what came back.
//
// Returns an error only for a problem with the inventory itself (a
// duplicate key, an address outside the mesh, an exhausted CIDR), because
// those make the whole plan wrong and distributing a wrong plan is worse
// than distributing nothing. A node that simply could not be reached is
// recorded in its NodeResult.Err and does not fail the pass.
//
// The returned inventory is the input with any newly allocated addresses
// and newly reported identities folded in. The caller persists it; this
// function deliberately has no store, so that the allocation decision and
// the persistence of it are separable and the decision stays testable.
func (c *Coordinator) Distribute(ctx context.Context, nodes []NodeInfo) ([]NodeInfo, DistributeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, DistributeResult{}, fmt.Errorf("distribute mesh config: %w", err)
	}

	inventory, err := AllocateAddresses(nodes, c.opts.MeshCIDR)
	if err != nil {
		return nil, DistributeResult{}, fmt.Errorf("distribute mesh config: %w", err)
	}
	c.applyObservedEndpoints(inventory)

	plans, err := PlanAll(inventory, c.opts)
	if err != nil {
		return nil, DistributeResult{}, fmt.Errorf("distribute mesh config: %w", err)
	}

	ordered := make([]NodeInfo, len(inventory))
	copy(ordered, inventory)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	result := DistributeResult{Nodes: make([]NodeResult, 0, len(ordered))}
	byID := make(map[string]int, len(inventory))
	for i, n := range inventory {
		byID[n.ID] = i
	}

	for _, n := range ordered {
		cfg := plans[n.ID]
		nr := NodeResult{NodeID: n.ID, Config: cfg}

		identity, applyErr := c.sink.ApplyMesh(ctx, n.ID, cfg)
		switch {
		case applyErr != nil:
			nr.Err = fmt.Errorf("apply mesh config on node %q: %w", n.ID, applyErr)
			c.logger.Warn("node did not accept its mesh config this pass",
				slog.String("node_id", n.ID),
				slog.Int("peers", len(cfg.Peers)),
				slog.String("error", applyErr.Error()))
		default:
			nr.Identity = identity
			if changed := c.foldIdentity(&inventory[byID[n.ID]], identity); changed {
				result.Updated = append(result.Updated, n.ID)
			}
		}
		result.Nodes = append(result.Nodes, nr)
	}

	sort.Strings(result.Updated)
	c.logger.Debug("mesh distribution pass complete",
		slog.Int("nodes", len(result.Nodes)),
		slog.Int("failed", len(result.Failed())),
		slog.Int("updated", len(result.Updated)))
	return inventory, result, nil
}

// applyObservedEndpoints overwrites each node's self-reported endpoint
// with the control plane's own observation where it has one. See
// NodeIdentity.Endpoint for why the observation wins: a NATted node's own
// view of its address is its private one, which no other node can reach.
func (c *Coordinator) applyObservedEndpoints(inventory []NodeInfo) {
	for i := range inventory {
		if observed, ok := c.observedEndpoint(inventory[i].ID); ok {
			inventory[i].Endpoint = observed
		}
	}
}

// foldIdentity merges what a node reported into the inventory entry for
// it, and reports whether anything changed.
//
// A node reporting a *different* public key than the one on file is
// treated as the new truth rather than as a conflict. That is the correct
// reading: the only way it happens is a node that lost its key file and
// generated a fresh one (a rebuilt machine, a wiped data directory), in
// which case the old key is genuinely dead and refusing to accept the new
// one would leave that node permanently unreachable with no operator
// action available short of deleting and re-enrolling it. It is logged at
// Warn because it is also what a node impersonation attempt would look
// like, and the mTLS identity behind the session (ADR 003) is what
// actually authorizes the claim, not this function.
func (c *Coordinator) foldIdentity(n *NodeInfo, id NodeIdentity) bool {
	changed := false

	if !id.PublicKey.IsZero() && id.PublicKey != n.PublicKey {
		if !n.PublicKey.IsZero() {
			c.logger.Warn("node reported a new public key, replacing the one on file",
				slog.String("node_id", n.ID))
		}
		n.PublicKey = id.PublicKey
		changed = true
	}

	// The node's self-reported endpoint is only used when the control
	// plane has not observed one, per NodeIdentity.Endpoint.
	if _, observed := c.observedEndpoint(n.ID); !observed && id.Endpoint != "" {
		if normalized, err := ParseEndpoint(id.Endpoint); err == nil && normalized != n.Endpoint {
			n.Endpoint = normalized
			changed = true
		} else if err != nil {
			c.logger.Warn("node reported an unusable endpoint, ignoring it",
				slog.String("node_id", n.ID), slog.String("error", err.Error()))
		}
	}
	return changed
}

// LocalSink is the ConfigSink for a node whose Mesh this process holds
// directly: the control plane's own node in every deployment, and every
// node in a single-node one.
//
// This is the in-process counterpart to internal/agent.Local, and exists
// for the same reason that type does (CLAUDE.md 4.3: single-node mode
// runs "in the same process as the control plane, communicating over an
// in-memory transport that implements the same interface"). The code path
// above it is identical whether there is one node or ten.
type LocalSink struct {
	nodeID string
	mesh   Mesh

	// privateKey is this node's own, held here and never anywhere else in
	// this package's control-plane-side types. It is injected into the
	// config on the way to Mesh.Apply, which is the one place a private
	// key is legitimately needed and the last possible moment to add it.
	privateKey Key
	publicKey  Key
	listenPort int
}

// NewLocalSink builds a sink that applies configs to mesh as nodeID,
// using privateKey as this node's identity.
func NewLocalSink(nodeID string, mesh Mesh, privateKey Key) (*LocalSink, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: local sink needs a node ID", ErrUnknownNode)
	}
	if mesh == nil {
		return nil, errors.New("network: local sink needs a mesh")
	}
	if privateKey.IsZero() {
		return nil, fmt.Errorf("network: local sink for node %q: %w", nodeID, ErrZeroKey)
	}
	pub, err := privateKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("network: local sink for node %q: %w", nodeID, err)
	}
	return &LocalSink{nodeID: nodeID, mesh: mesh, privateKey: privateKey, publicKey: pub}, nil
}

// ApplyMesh applies cfg to the local mesh and reports this node's
// identity back.
//
// A config addressed to a different node is an error rather than being
// silently applied: a sink that accepted anyone's config would apply
// another node's peer list, and every peer entry in it would be wrong
// (including, worst of all, one for this node itself).
func (s *LocalSink) ApplyMesh(ctx context.Context, nodeID string, cfg DeviceConfig) (NodeIdentity, error) {
	if nodeID != s.nodeID {
		return NodeIdentity{}, fmt.Errorf("%w: local sink is node %q, was handed config for %q",
			ErrUnknownNode, s.nodeID, nodeID)
	}

	cfg.PrivateKey = s.privateKey
	if err := s.mesh.Apply(ctx, cfg); err != nil {
		return NodeIdentity{}, err
	}

	port := cfg.ListenPort
	if st, err := s.mesh.Status(ctx); err == nil && st.ListenPort > 0 {
		port = st.ListenPort
	}
	s.listenPort = port

	return NodeIdentity{
		PublicKey:  s.publicKey,
		ListenPort: port,
		// No endpoint: the control plane's own node has no meaningful
		// self-reported endpoint (it is the thing others dial), and
		// guessing one from a local interface address would produce
		// something no remote node can reach.
	}, nil
}

// PublicKey reports this node's public key.
func (s *LocalSink) PublicKey() Key { return s.publicKey }

// staleAfter is how long a peer may go without a handshake before
// PeerStatus.Healthy calls it unreachable. WireGuard's own rekey interval
// is 120 seconds on an active session, so three minutes is one missed
// rekey plus margin: long enough that an idle-but-fine link is not
// flagged, short enough that a genuinely dead peer surfaces within a few
// minutes rather than at the next deploy.
const staleAfter = 3 * time.Minute

// UnhealthyPeers returns the peers in st that have not handshaken
// recently enough, for a caller building a node-health view (TASKS.md
// 3.7) or logging why a cross-node call is failing.
func UnhealthyPeers(st Status, now time.Time) []PeerStatus {
	var out []PeerStatus
	for _, p := range st.Peers {
		if !p.Healthy(now, staleAfter) {
			out = append(out, p)
		}
	}
	return out
}

// MeshAddresses extracts the assigned mesh addresses from an inventory,
// for a caller that needs the set of addresses currently in use (the
// store, persisting them; a status endpoint, showing them).
func MeshAddresses(nodes []NodeInfo) []netip.Addr {
	out := make([]netip.Addr, 0, len(nodes))
	for _, n := range nodes {
		if n.Address.IsValid() {
			out = append(out, n.Address)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	return out
}
