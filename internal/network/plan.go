package network

// This file: the full-mesh decision. Given what the control plane knows
// about every node, produce the DeviceConfig for one specific node.

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
