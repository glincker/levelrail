package network

// This file: the desired-state value types (what the control plane knows
// about a node, what one node's device should look like) and the
// validation that runs before any of it reaches a real device.
//
// Validation lives here and not inside Device on purpose. By the time a
// DeviceConfig reaches a WireGuard device, a bad one has already been
// distributed to every node in the fleet; catching it at the point the
// control plane assembles it, from data nodes reported about themselves,
// is the only place it can be caught before it spreads.

import (
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NodeInfo is everything the control plane knows about one node for mesh
// purposes: enough to make every *other* node able to reach it.
//
// PublicKey and Endpoint are reported by the node itself (only the node
// can know its own key, and only the node's own view plus the control
// plane's observation of where it dialed from can establish a usable
// endpoint). Address is assigned by the control plane, because address
// allocation is the one part that cannot be decided locally without
// collisions. That split is the whole reason distribution is a two-way
// exchange rather than a one-way push; see Coordinator.
type NodeInfo struct {
	// ID is store.Node.ID. The empty string is not valid here, unlike in
	// desired_services.node_id where "" means "the control plane's own
	// local node" (migrations/0009_node_placement.sql): a mesh has no
	// implicit node, every participant including the control plane's own
	// has a real row and a real key.
	ID   string
	Name string

	// PublicKey is this node's WireGuard public key. Zero until the node
	// has reported one, which is a normal transient state for a node
	// that enrolled but has not yet come up on the mesh, not an error:
	// Plan skips such a node as a peer rather than failing the whole
	// plan, so one not-yet-ready node cannot stop the rest of the fleet
	// from meshing.
	PublicKey Key

	// Endpoint is "host:port", the UDP address other nodes dial to reach
	// this one. May be empty for a node behind NAT that has never been
	// observed dialing out: WireGuard learns a peer's endpoint from its
	// first inbound handshake, so a NATted node is reachable as soon as
	// it initiates, which (ADR 003's reverse-dial design) it always
	// does. An empty endpoint therefore means "wait for it to talk to
	// us," not "unreachable."
	Endpoint string

	// Address is this node's mesh IP, assigned by AllocateAddress and
	// persisted by the control plane. Invalid (the zero netip.Addr)
	// until assigned, same "not ready yet" meaning as a zero PublicKey.
	Address netip.Addr
}

// Ready reports whether this node has everything Plan needs to make it a
// reachable peer for other nodes.
func (n NodeInfo) Ready() bool {
	return n.ID != "" && !n.PublicKey.IsZero() && n.Address.IsValid()
}

// PeerConfig is one entry in a node's WireGuard device configuration:
// another node, as seen from this one.
type PeerConfig struct {
	// NodeID is not part of WireGuard's own model at all. It is carried
	// so every log line about a peer can name the resource by the ID the
	// rest of this codebase uses (CLAUDE.md 7: "every log line that
	// describes a resource includes its ID"), rather than by a base64
	// key that matches nothing else in the database.
	NodeID    string
	PublicKey Key
	Endpoint  string

	// AllowedIPs is what traffic this peer is permitted to send and
	// what destinations route to it. For a full mesh of single-address
	// nodes this is exactly one /32 per peer, never 0.0.0.0/0: a
	// default route through a peer would make every node a potential
	// exit node for every other, which is a materially different
	// security posture than "these machines can reach each other" and
	// not something ADR 006 asks for.
	AllowedIPs []netip.Prefix

	PersistentKeepalive time.Duration
}

// DeviceConfig is the complete desired WireGuard state for exactly one
// node. Complete, not incremental: see Mesh.Apply on why there is no
// delta form.
//
// PrivateKey is the local node's own, and is the one field in this
// package that must never be logged, serialized to the control plane, or
// included in any status response. It is only ever populated on the node
// the config is for, by that node, immediately before Apply; a
// DeviceConfig that crossed the wire from the control plane always has a
// zero PrivateKey and the receiving node fills it in from its own local
// key file.
type DeviceConfig struct {
	NodeID     string
	PrivateKey Key

	// Address is this node's own mesh address with the mesh CIDR's
	// prefix length (e.g. 10.181.0.3/16, not /32), because that prefix
	// is what tells the host routing table to send the whole mesh range
	// out the WireGuard interface.
	Address    netip.Prefix
	ListenPort int
	Peers      []PeerConfig
}

// LogValue implements slog.LogValuer so a DeviceConfig can be logged
// without leaking PrivateKey. Without this, any structured log line that
// passed a DeviceConfig would print 32 bytes of private key material into
// the log file; with it, the private key is structurally unreachable from
// the logging path rather than merely omitted by convention at each call
// site. Same "make the safe thing the only thing" reasoning
// internal/secrets applies to env values.
func (c DeviceConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("node_id", c.NodeID),
		slog.String("address", c.Address.String()),
		slog.Int("listen_port", c.ListenPort),
		slog.Int("peers", len(c.Peers)),
	)
}

// PeerByNodeID returns the peer entry for nodeID, if this config has one.
func (c DeviceConfig) PeerByNodeID(nodeID string) (PeerConfig, bool) {
	for _, p := range c.Peers {
		if p.NodeID == nodeID {
			return p, true
		}
	}
	return PeerConfig{}, false
}

// ValidateInventory checks a whole node inventory for the collisions that
// would make a mesh silently misroute rather than loudly fail, and
// returns the first problem it finds.
//
// Only nodes that are Ready are checked: a node that has enrolled but not
// yet reported a key or been assigned an address is a normal in-progress
// state (it simply is not a peer yet), and failing the whole inventory
// for it would mean one half-enrolled node stops the entire fleet from
// meshing.
//
// meshCIDR bounds every node address. It is checked rather than assumed
// because addresses are persisted values that outlive any single run: an
// operator who narrows APP_MESH_CIDR after nodes were allocated
// addresses from a wider one would otherwise get AllowedIPs entries
// pointing outside the range they meant to delegate, and would find out
// from a routing symptom rather than an error.
func ValidateInventory(nodes []NodeInfo, meshCIDR netip.Prefix) error {
	if err := validateCIDR(meshCIDR); err != nil {
		return err
	}

	byKey := make(map[Key]string, len(nodes))
	byAddr := make(map[netip.Addr]string, len(nodes))
	seenID := make(map[string]struct{}, len(nodes))

	for _, n := range nodes {
		if n.ID == "" {
			return fmt.Errorf("network: inventory contains a node with no ID")
		}
		if _, dup := seenID[n.ID]; dup {
			return fmt.Errorf("network: inventory lists node %q twice", n.ID)
		}
		seenID[n.ID] = struct{}{}

		if !n.Ready() {
			continue
		}

		if other, dup := byKey[n.PublicKey]; dup {
			return fmt.Errorf("%w: %q and %q", ErrDuplicatePublicKey, other, n.ID)
		}
		byKey[n.PublicKey] = n.ID

		if other, dup := byAddr[n.Address]; dup {
			return fmt.Errorf("%w: %q and %q both claim %s", ErrDuplicateAddress, other, n.ID, n.Address)
		}
		byAddr[n.Address] = n.ID

		if !meshCIDR.Contains(n.Address) {
			return fmt.Errorf("%w: node %q has %s, mesh is %s", ErrAddressOutsideMesh, n.ID, n.Address, meshCIDR)
		}
	}
	return nil
}

// validateCIDR rejects the mesh CIDRs that would produce a broken plan.
// IPv4 only, deliberately: the mesh is a private range the platform hands
// out itself, so there is no interoperability reason to support both
// families, and supporting one means AllocateAddress's arithmetic and
// the /32 host-prefix convention in Plan each have exactly one form to be
// correct in rather than two.
func validateCIDR(cidr netip.Prefix) error {
	switch {
	case !cidr.IsValid():
		return fmt.Errorf("%w: unset", ErrInvalidMeshCIDR)
	case !cidr.Addr().Is4():
		return fmt.Errorf("%w: %s is not IPv4", ErrInvalidMeshCIDR, cidr)
	case cidr.Addr() != cidr.Masked().Addr():
		return fmt.Errorf("%w: %s has host bits set, want %s", ErrInvalidMeshCIDR, cidr, cidr.Masked())
	case cidr.Bits() > 30:
		// A /31 or /32 has no usable host range at all once the network
		// address is reserved; a mesh needs at least two addresses to be
		// a mesh.
		return fmt.Errorf("%w: %s is too small to hold a mesh", ErrInvalidMeshCIDR, cidr)
	}
	return nil
}

// ParseEndpoint validates a "host:port" endpoint string and returns it
// normalized. Returns an empty string with no error for an empty input:
// "no endpoint yet" is a legitimate state (see NodeInfo.Endpoint), not a
// parse failure.
func ParseEndpoint(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	host, port, err := splitHostPort(s)
	if err != nil {
		return "", fmt.Errorf("network: parse endpoint %q: %w", s, err)
	}
	if host == "" {
		return "", fmt.Errorf("network: parse endpoint %q: empty host", s)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("network: parse endpoint %q: port must be 1-65535", s)
	}
	// An IP literal round-trips through netip so an address written in a
	// non-canonical form ("10.0.000.1") does not end up as a distinct
	// string from the canonical one elsewhere. A DNS name is left alone.
	if addr, addrErr := netip.ParseAddr(host); addrErr == nil {
		return netip.AddrPortFrom(addr, uint16(n)).String(), nil
	}
	return host + ":" + port, nil
}

// splitHostPort splits "host:port" and "[v6]:port" without net.SplitHostPort's
// dependency on package net, which this package otherwise has no need of
// (netip covers every address operation here).
func splitHostPort(s string) (host, port string, err error) {
	if strings.HasPrefix(s, "[") {
		end := strings.LastIndex(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return "", "", fmt.Errorf("malformed bracketed address")
		}
		return s[1:end], s[end+2:], nil
	}
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", "", fmt.Errorf("missing port")
	}
	if strings.Contains(s[:i], ":") {
		return "", "", fmt.Errorf("IPv6 address must be bracketed")
	}
	return s[:i], s[i+1:], nil
}

// sortPeers orders peers by node ID so a DeviceConfig produced from the
// same inventory twice is byte-identical both times. That determinism is
// what lets Apply and its tests compare a freshly planned config against
// the last applied one to decide whether anything actually changed,
// instead of re-applying an unchanged config on every reconcile tick.
func sortPeers(peers []PeerConfig) {
	sort.Slice(peers, func(i, j int) bool { return peers[i].NodeID < peers[j].NodeID })
}
