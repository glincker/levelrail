// Package meshpath answers one question for the ingress and application
// controllers: can this control plane reach a remote node's published
// ports over the WireGuard mesh right now, and on which address.
package meshpath

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StaleAfter is how old a WireGuard handshake may be before the path counts
// as down. Persistent keepalive keeps a healthy peer well inside it.
const StaleAfter = 3 * time.Minute

// Path is the mesh route from this control plane to one node.
type Path struct {
	// Local is true for the control plane's own node, which needs no mesh.
	Local bool
	// Usable is true when Address can be dialed and bound to right now.
	Usable bool
	// Address is the node's mesh IP, set only when Usable.
	Address string
	// Reason says why the path is not usable, with the next action.
	Reason string
}

// Resolver reports the mesh path to a node.
type Resolver interface {
	Path(ctx context.Context, nodeID string) (Path, error)
}

// ValidateBindIP is the single guard every mesh bind goes through: only a
// concrete unicast address may be bound, never a wildcard, loopback or
// multicast one, so a mesh bind can never widen a port to every interface.
func ValidateBindIP(ip string) error {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("meshpath: %q is not an IP address: %w", ip, err)
	}
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() {
		return fmt.Errorf("meshpath: %s is not a mesh unicast address", ip)
	}
	return nil
}

// StatusReader is the slice of network.Mesh the tracker needs.
type StatusReader interface {
	Status(ctx context.Context) (network.Status, error)
}

// NodeStore is the slice of the store the tracker needs.
type NodeStore interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)
}

// Tracker derives Paths from the control plane's live WireGuard device.
type Tracker struct {
	localID string
	mesh    StatusReader
	nodes   NodeStore
	now     func() time.Time
}

// NewTracker builds a Tracker. mesh may be nil when mesh networking is off.
func NewTracker(localID string, mesh StatusReader, nodes NodeStore) *Tracker {
	return &Tracker{localID: localID, mesh: mesh, nodes: nodes, now: time.Now}
}

// Path implements Resolver.
func (t *Tracker) Path(ctx context.Context, nodeID string) (Path, error) {
	if nodeID == "" || nodeID == t.localID {
		return Path{Local: true}, nil
	}
	if t.mesh == nil {
		return Path{Reason: "mesh networking is off on this control plane: set APP_MESH_ENABLED=1 and restart it"}, nil
	}
	node, err := t.nodes.GetNode(ctx, nodeID)
	if err != nil {
		return Path{}, fmt.Errorf("meshpath: load node %q: %w", nodeID, err)
	}
	if node.MeshAddress == "" || node.MeshPublicKey == "" {
		return Path{Reason: "the node has not joined the mesh yet: check `levelrail-cli nodes mesh`"}, nil
	}
	addr, err := netip.ParseAddr(node.MeshAddress)
	if err != nil {
		return Path{Reason: fmt.Sprintf("the node's mesh address %q is invalid: check `levelrail-cli nodes mesh`", node.MeshAddress)}, nil
	}
	st, err := t.mesh.Status(ctx)
	if err != nil {
		return Path{}, fmt.Errorf("meshpath: read mesh status: %w", err)
	}
	if !st.Address.IsValid() {
		return Path{Reason: "this control plane's WireGuard device is not up (it needs root and the wireguard kernel module): check `levelrail-cli nodes mesh`"}, nil
	}
	if !st.Address.Masked().Contains(addr) {
		return Path{Reason: fmt.Sprintf("the node's mesh address %s is outside the mesh network %s", addr, st.Address.Masked())}, nil
	}
	if err := ValidateBindIP(addr.String()); err != nil {
		return Path{Reason: err.Error()}, nil //nolint:nilerr // an unusable address is a Path state, not a failure
	}
	for _, p := range st.Peers {
		if p.PublicKey.String() != node.MeshPublicKey {
			continue
		}
		if !p.Healthy(t.now(), StaleAfter) {
			return Path{Reason: "no recent WireGuard handshake with the node: check that UDP 51820 is open between the hosts, then `levelrail-cli nodes mesh`"}, nil
		}
		return Path{Usable: true, Address: addr.String()}, nil
	}
	return Path{Reason: "the control plane has no WireGuard peer for the node yet, the mesh reconciler will add it on its next pass"}, nil
}

var _ Resolver = (*Tracker)(nil)
