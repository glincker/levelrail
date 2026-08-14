// Package network is ADR 006's WireGuard mesh: the abstraction that ADR
// 006 says "has to be designed in Phase 1 as an interface, not a concrete
// WireGuard dependency" and that, until TASKS.md 3.4, did not exist at
// all (the directory the project's repo layout reserves for it was empty).
//
// The split this package draws, and why:
//
//   - Everything that decides *what the mesh should look like* (Plan,
//     AllocateAddress, BuildRecords, Coordinator) is pure Go over plain
//     values. It needs no root, no kernel module, no TUN device, and no
//     network. It is therefore table-driven testable, which matters
//     because it is where the actual product decisions live: full mesh
//     rather than hub-and-spoke, which node an app's DNS name points at,
//     what happens when a node reports a key that collides with another.
//   - Everything that *makes the kernel agree with that decision*
//     (Device, uapi.go, detect.go) is deliberately thin, and is the only
//     part that touches wireguard-go, privileged syscalls, or /sys. It
//     cannot be unit tested without root; what it can do is be small
//     enough that a human reviewing it can see it is a faithful
//     translation of a DeviceConfig, and no more.
//
// That boundary is the same one internal/docker already draws between
// Runtime (the narrow surface controllers depend on) and Client (the
// part that actually talks to a daemon), and it exists here for the same
// reason: the interesting failure modes are in the decisions, not in the
// syscalls.
//
// This package deliberately does not import internal/brand. The DNS zone
// is derived from a short name passed in by the caller (see Zone), so
// nothing here embeds a product name, matching the rule that no product
// name string appears anywhere in source, and the package stays testable
// without loading brand.yaml.
package network

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

// Backend names which WireGuard implementation a Mesh is running on.
// Not a bool "kernel or not": Disabled is a real, expected state (a
// single-node deployment never brings a mesh up at all, ADR 006's
// "single-node deployments do not pay any WireGuard cost"), and the
// value is surfaced in Status so an operator can tell the fast path from
// the portable one without guessing.
type Backend string

// The three states a Mesh's backend can be in.
const (
	// BackendKernel is the in-kernel WireGuard module: the fast path,
	// used when the module is present and this process can configure it.
	BackendKernel Backend = "kernel"

	// BackendUserspace is wireguard-go: ADR 006's portability path, for
	// hosts where loading a kernel module is not an option.
	BackendUserspace Backend = "userspace"

	// BackendDisabled is no mesh at all. Everything still resolves and
	// reconciles; nothing is encrypted or routed between machines,
	// because there is only one machine.
	BackendDisabled Backend = "disabled"
)

// DefaultListenPort is the UDP port a node's WireGuard device listens on
// unless an operator overrides it. 51820 is WireGuard's own conventional
// port; using the convention means an operator's existing firewall
// muscle memory applies unchanged.
const DefaultListenPort = 51820

// DefaultKeepalive is the persistent-keepalive interval set on every
// peer. Non-zero by default on purpose: ADR 003 already commits to nodes
// that may sit behind NAT ("works behind NAT and residential
// connections"), and a NAT mapping with no traffic through it expires in
// well under a minute on consumer hardware. 25s is WireGuard's own
// documented recommendation for exactly this case.
const DefaultKeepalive = 25 * time.Second

// Mesh is the abstraction ADR 006 requires: everything above it knows
// "make this node's networking match this desired state" and nothing
// about WireGuard, kernel modules, or TUN devices.
//
// Apply is level-triggered and idempotent, the same contract every
// reconciler in this codebase already has: reconcilers are idempotent,
// level-triggered, and safe to interrupt, never edge-triggered. Callers
// hand it the complete desired peer set every time, never a delta: there
// is deliberately no AddPeer/RemovePeer pair, because a delta API makes
// the caller responsible for tracking what it already sent, which is
// precisely the edge-triggered design that contract rules out.
type Mesh interface {
	// Apply converges this node's WireGuard device on cfg. Peers present
	// on the device but absent from cfg are removed; peers in cfg that
	// the device does not have are added; a peer whose endpoint or
	// allowed IPs changed is updated in place, not torn down and
	// recreated (tearing down would drop the session key and force a new
	// handshake for no reason).
	Apply(ctx context.Context, cfg DeviceConfig) error

	// Status reports what the device currently looks like, including
	// per-peer last-handshake times. This is what makes "peer
	// unreachable" observable rather than silent: a peer configured
	// minutes ago with a zero LastHandshake has never completed a
	// handshake, which is the mesh's equivalent of a failing health
	// check.
	Status(ctx context.Context) (Status, error)

	// Close tears the device down. Safe to call more than once.
	Close() error
}

// Status is one node's observed mesh state, the "observed" half of the
// reconcile pair whose "desired" half is DeviceConfig.
type Status struct {
	Backend    Backend
	Interface  string
	PublicKey  Key
	ListenPort int
	Address    netip.Prefix
	Peers      []PeerStatus
}

// PeerStatus is one peer as the local device currently sees it.
type PeerStatus struct {
	// NodeID is carried through from the PeerConfig that created this
	// peer so a caller can log a peer by the ID everything else in this
	// codebase logs resources by, rather than by a base64 public key.
	// WireGuard itself has no concept of a node ID, so this is empty for
	// any peer the device has that no current DeviceConfig named.
	NodeID    string
	PublicKey Key
	Endpoint  string

	// LastHandshake is zero when no handshake has ever completed with
	// this peer, which is exactly the "configured but unreachable" case:
	// a peer entry can exist indefinitely without the other end ever
	// answering.
	LastHandshake time.Time
	TransferRx    int64
	TransferTx    int64
}

// Healthy reports whether this peer has completed a handshake recently
// enough to be considered reachable. WireGuard rekeys roughly every two
// minutes on an active session, so a handshake older than staleAfter
// means traffic has not flowed; a zero LastHandshake means it never has.
func (p PeerStatus) Healthy(now time.Time, staleAfter time.Duration) bool {
	if p.LastHandshake.IsZero() {
		return false
	}
	return now.Sub(p.LastHandshake) <= staleAfter
}

// The errors this package returns. Every one of them is a real, reachable
// state rather than a defensive check: a control plane assembling a mesh
// from node-reported values is assembling it from data it did not
// generate itself, so every one of these is something a misbehaving or
// misconfigured node can actually cause.
var (
	// ErrUnknownNode is returned by Plan when the node it was asked to
	// plan for is not in the inventory it was given.
	ErrUnknownNode = errors.New("network: node not present in the mesh inventory")

	// ErrZeroKey is returned when a node's public key is the all-zero
	// value. That is never a legitimate key; it is what an unset field
	// looks like, and treating it as a real key would configure a peer
	// that can never handshake while looking fine in the config.
	ErrZeroKey = errors.New("network: node has no public key")

	// ErrDuplicatePublicKey is returned when two nodes report the same
	// public key. WireGuard identifies peers solely by public key, so
	// two nodes sharing one is not a duplicate row to be tolerated, it
	// makes the mesh silently misroute: whichever peer entry was written
	// last wins for both nodes.
	ErrDuplicatePublicKey = errors.New("network: two nodes share a public key")

	// ErrDuplicateAddress is returned when two nodes claim the same mesh
	// address. Same class of failure as a duplicate key, one layer up.
	ErrDuplicateAddress = errors.New("network: two nodes share a mesh address")

	// ErrAddressOutsideMesh is returned when a node's mesh address is
	// not inside the configured mesh CIDR, which would produce an
	// AllowedIPs entry routing traffic somewhere the operator never
	// delegated to this mesh.
	ErrAddressOutsideMesh = errors.New("network: node address is outside the mesh CIDR")

	// ErrNoAddressAvailable is returned by AllocateAddress when the mesh
	// CIDR is fully allocated.
	ErrNoAddressAvailable = errors.New("network: no free address left in the mesh CIDR")

	// ErrInvalidMeshCIDR is returned when the configured mesh CIDR is
	// unset, not canonical, or not IPv4.
	ErrInvalidMeshCIDR = errors.New("network: invalid mesh CIDR")

	// ErrMeshClosed is returned by a Mesh whose Close has already been
	// called.
	ErrMeshClosed = errors.New("network: mesh is closed")
)
