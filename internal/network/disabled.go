package network

// This file: the Mesh implementation for a node that has no mesh.
//
// It exists as a real named type rather than a nil Mesh plus nil checks
// at every call site for the same reason internal/agent.Local exists
// rather than relying on Go's structural typing: it makes "this node
// deliberately has no mesh" a statement in the code instead of an
// absence, and it means no caller has to branch. ADR 006's consequence
// that "single-node deployments do not pay any WireGuard cost at all" is
// this type.

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
)

// Disabled is a Mesh that accepts configuration and does nothing with it.
//
// Apply is not an error: a single-node control plane still runs the same
// coordinator code path as a ten-node one, and making the zero-peer case
// an error would mean the mesh coordinator needs a "is the mesh on"
// branch that the multi-node path does not exercise, which is precisely
// the kind of untested-in-the-common-case branch that fails when it
// finally runs.
//
// Applying a config with peers *is* recorded in Status, so an operator
// looking at a node whose services cannot reach another node sees "mesh
// backend: disabled, 3 peers configured" rather than an empty status
// indistinguishable from a working one.
type Disabled struct {
	reason string

	mu     sync.Mutex
	closed bool
	last   DeviceConfig
}

// NewDisabled returns a Mesh that does nothing, recording reason so
// Status can explain why. reason comes from DetectResult.Reason when the
// mesh is disabled by detection, or from the caller when it is disabled
// by configuration (a single-node deployment).
func NewDisabled(reason string) *Disabled {
	return &Disabled{reason: reason}
}

// Reason reports why this mesh is disabled.
func (d *Disabled) Reason() string { return d.reason }

// Apply records cfg without configuring anything.
func (d *Disabled) Apply(ctx context.Context, cfg DeviceConfig) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("network: apply mesh config for node %q: %w", cfg.NodeID, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("network: apply mesh config for node %q: %w", cfg.NodeID, ErrMeshClosed)
	}
	d.last = cfg
	return nil
}

// Status reports the disabled backend and whatever config was last
// applied, with no peer state (there are no peers, only peer intentions).
func (d *Disabled) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, fmt.Errorf("network: mesh status: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Status{}, fmt.Errorf("network: mesh status: %w", ErrMeshClosed)
	}

	st := Status{
		Backend:    BackendDisabled,
		ListenPort: d.last.ListenPort,
		Address:    d.last.Address,
		Peers:      make([]PeerStatus, 0, len(d.last.Peers)),
	}
	for _, p := range d.last.Peers {
		st.Peers = append(st.Peers, PeerStatus{
			NodeID:    p.NodeID,
			PublicKey: p.PublicKey,
			Endpoint:  p.Endpoint,
			// LastHandshake stays zero: no handshake has happened or can
			// happen, which PeerStatus.Healthy already reads as
			// unreachable. That is the truthful answer, not a missing one.
		})
	}
	return st, nil
}

// Close marks this mesh closed. Idempotent.
func (d *Disabled) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.last = DeviceConfig{}
	return nil
}

// LocalAddress reports the mesh address this node was last configured
// with, or the zero Prefix if none. Used by the DNS layer, which needs to
// answer for locally-placed services with an address that is reachable
// even when the mesh itself is disabled.
func (d *Disabled) LocalAddress() netip.Prefix {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last.Address
}
