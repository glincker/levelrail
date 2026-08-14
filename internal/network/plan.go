package network

// This file: the full-mesh decision. Given what the control plane knows
// about every node, produce the DeviceConfig for one specific node.
//
// This is the smallest possible expression of ADR 006's central choice,
// and it is deliberately kept that way so the choice is auditable in one
// function: "every other ready node becomes a direct peer of this one."
// The rejected alternative (hub-and-spoke, where each node peers only
// with the control plane and the control plane forwards) is ruled out by
// ADR 006 in one line, but it is worth being explicit about why, because
// it is the shape that falls out naturally if nobody decides otherwise:
//
//   - Hub-and-spoke doubles the latency of every cross-node call and puts
//     the whole fleet's east-west traffic through one process. That is
//     exactly the "gets heavier as resource count grows" failure CLAUDE.md
//     1 and 4.8 name as the reason this project exists at all.
//   - It makes the control plane a hard dependency of the data path. A
//     control plane restart, which is otherwise a completely safe
//     operation (every reconciler is level-triggered and resumes), would
//     sever every inter-service connection in the fleet.
//   - It gains nothing WireGuard does not already do: a full mesh of N
//     nodes is N*(N-1)/2 peer relationships, but each node only holds
//     N-1 peer entries, and a peer entry is a public key and an
//     endpoint. At the 1-to-10 machines CLAUDE.md 1 targets, that is at
//     most nine entries per node.

import (
	"fmt"
	"net/netip"
	"time"
)

// PlanOptions are the fleet-wide settings a plan is computed against, as
// opposed to the per-node facts in the inventory.
type PlanOptions struct {
	// MeshCIDR is the private range every node's address comes from. It
	// determines both the prefix length on DeviceConfig.Address (so the
	// host routes the whole mesh out the WireGuard interface) and the
	// bound ValidateInventory checks every node address against.
	MeshCIDR netip.Prefix

	// ListenPort is the UDP port each node's device listens on. Zero
	// means DefaultListenPort.
	ListenPort int

	// Keepalive is the persistent-keepalive interval set on every peer.
	// Zero means DefaultKeepalive; explicitly disabling it requires a
	// negative value, so "I did not set this field" and "I want no
	// keepalive" are distinguishable. See DefaultKeepalive for why the
	// default is non-zero.
	Keepalive time.Duration
}

func (o PlanOptions) listenPort() int {
	if o.ListenPort == 0 {
		return DefaultListenPort
	}
	return o.ListenPort
}

func (o PlanOptions) keepalive() time.Duration {
	switch {
	case o.Keepalive == 0:
		return DefaultKeepalive
	case o.Keepalive < 0:
		return 0
	default:
		return o.Keepalive
	}
}

// Plan computes the DeviceConfig for selfID from the full node inventory.
//
// The returned config's PrivateKey is deliberately zero: this runs on the
// control plane, which does not have and must never have any node's
// private key (see DeviceConfig's own doc comment). The node fills it in
// from its own local key file before calling Apply.
//
// A node in the inventory that is not Ready (no public key yet, no
// address yet) is skipped as a peer rather than treated as an error. Node
// enrollment (ADR 003) and mesh readiness are separate events, and a
// fleet where one machine enrolled thirty seconds ago must still be able
// to mesh the machines that are ready. The consequence is that the mesh
// converges over successive passes rather than atomically, which is the
// same level-triggered convergence every other controller in this
// codebase already has (CLAUDE.md 4.2), not a weaker guarantee.
//
// selfID itself does not need to be Ready. A node that has not yet
// reported a key still gets a valid plan naming its peers, which is
// exactly what it needs on its very first pass: it is the act of applying
// that plan that produces the key it will report back.
func Plan(selfID string, nodes []NodeInfo, opts PlanOptions) (DeviceConfig, error) {
	if selfID == "" {
		return DeviceConfig{}, fmt.Errorf("%w: no node ID given", ErrUnknownNode)
	}
	if err := ValidateInventory(nodes, opts.MeshCIDR); err != nil {
		return DeviceConfig{}, fmt.Errorf("plan mesh for node %q: %w", selfID, err)
	}

	var self NodeInfo
	found := false
	for _, n := range nodes {
		if n.ID == selfID {
			self, found = n, true
			break
		}
	}
	if !found {
		return DeviceConfig{}, fmt.Errorf("%w: %q", ErrUnknownNode, selfID)
	}

	cfg := DeviceConfig{
		NodeID:     selfID,
		ListenPort: opts.listenPort(),
		Peers:      make([]PeerConfig, 0, max(len(nodes)-1, 0)),
	}
	if self.Address.IsValid() {
		cfg.Address = netip.PrefixFrom(self.Address, opts.MeshCIDR.Bits())
	}

	for _, n := range nodes {
		if n.ID == selfID || !n.Ready() {
			continue
		}
		cfg.Peers = append(cfg.Peers, PeerConfig{
			NodeID:    n.ID,
			PublicKey: n.PublicKey,
			Endpoint:  n.Endpoint,
			// Exactly one /32 per peer: this peer owns precisely its own
			// mesh address and routes nothing else. See PeerConfig.AllowedIPs
			// for why a wider prefix is not used.
			AllowedIPs:          []netip.Prefix{hostPrefix(n.Address)},
			PersistentKeepalive: opts.keepalive(),
		})
	}
	sortPeers(cfg.Peers)
	return cfg, nil
}

// PlanAll computes every node's DeviceConfig in one pass, validating the
// inventory once instead of once per node. This is what the Coordinator
// calls: distributing configs one node at a time through repeated Plan
// calls would revalidate the same inventory N times and, worse, could
// distribute a partially-planned mesh if validation failed partway
// through.
func PlanAll(nodes []NodeInfo, opts PlanOptions) (map[string]DeviceConfig, error) {
	if err := ValidateInventory(nodes, opts.MeshCIDR); err != nil {
		return nil, fmt.Errorf("plan mesh: %w", err)
	}
	out := make(map[string]DeviceConfig, len(nodes))
	for _, n := range nodes {
		cfg, err := Plan(n.ID, nodes, opts)
		if err != nil {
			return nil, err
		}
		out[n.ID] = cfg
	}
	return out, nil
}
