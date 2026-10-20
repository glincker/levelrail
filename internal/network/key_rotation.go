package network

// This file: operator-triggered WireGuard key rotation.
//
// The constraint that shapes everything here: a WireGuard device has
// exactly one private key at a time (device.IpcSet's own "private_key="
// line replaces it outright, see EncodeUAPI). There is no way to make a
// single device answer to both an old and a new key at once, the way a
// TLS cert rotation can serve two certs from two listeners during a
// cutover window. So "rotate this node's key with zero packet loss"
// would require standing up a second device with a second identity and
// running both until every peer has switched, which is a materially
// bigger change (a second TUN interface, a second listen port, a
// decision about which one carries real traffic during the overlap) than
// this pass is willing to risk shipping without a live multi-node rig to
// prove it against.
//
// What ships instead, and it is a deliberate simplification, not a
// silently cut corner: rotation is "rotate now." RotateKey generates a
// fresh keypair, makes it the node's live identity immediately, and
// returns. That is the same event that already happens, uncontrolled,
// when a node's data directory is wiped and it comes back up with a
// freshly generated key (see Coordinator.foldIdentity's own doc comment:
// "the only way it happens is a node that lost its key file... the old
// key is genuinely dead"). This codebase already accepts that case's
// transient partition as ordinary, converging over the next couple of
// reconcile passes the same way any other inventory change does. What
// this file adds on top is the part that case does not have: a
// deliberate, observable, resumable operation instead of an implicit
// side effect, with RotationStatus making "has the fleet actually caught
// up yet" a real answer instead of a guess.
//
// A rotation is "confirmed" once one full Distribute pass completes with
// no failed nodes after the rotation started: that is the strongest
// signal this package can produce without inventing a per-peer
// acknowledgement protocol, because Coordinator's only view of a remote
// node is whether ApplyMesh succeeded, and a pass with zero failures
// means every node in the fleet just accepted its current peer list,
// including whatever entry names this node's new key. It is not a
// per-peer handshake proof (that lives one layer down, in a live
// Mesh.Status() read against the UAPI, which is a per-node local fact
// Coordinator has no way to reach for a remote node); it is the best
// fleet-wide signal available at this layer, and it is exactly what an
// operator watching a rotation needs: "is this done, or is something
// still stuck."

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// KeyRotator is implemented by a ConfigSink that can also rotate the key
// of the node it applies configs to. LocalSink is the only implementation
// today; a future gRPC sink (see ConfigSink's own doc comment on why that
// transport does not exist yet) would implement it the same way it will
// implement ApplyMesh, by forwarding the request over the agent Session
// stream to the node that holds the private key.
//
// A separate interface rather than a method on ConfigSink itself: not
// every sink needs to support rotation (a read-only or test sink has no
// reason to), and Coordinator.RotateKey type-asserts for it rather than
// widening ConfigSink's own contract for every implementation that will
// ever exist.
type KeyRotator interface {
	// RotateKey generates a fresh keypair for nodeID, makes it that
	// node's live WireGuard identity, and returns the key it replaced
	// alongside the new one. An error means the node was not rotated;
	// same "one node's problem, not the fleet's" contract ApplyMesh's own
	// doc comment establishes, left to the caller to decide what to do
	// with.
	RotateKey(ctx context.Context, nodeID string) (RotationResult, error)
}

// RotationResult is the outcome of one RotateKey call.
type RotationResult struct {
	NodeID       string
	OldPublicKey Key
	NewPublicKey Key
}

// RotationStatus is a Coordinator's record of one rotation, for a caller
// (the mesh status API) that wants to tell an operator whether a
// requested rotation has actually finished propagating.
type RotationStatus struct {
	NodeID       string
	OldPublicKey Key
	NewPublicKey Key
	StartedAt    time.Time

	// Confirmed is true once a Distribute pass has completed with no
	// failed nodes since StartedAt. False does not mean something is
	// wrong: the very next pass after RotateKey returns is almost always
	// what flips this, so an operator watching this field typically sees
	// it go true within one reconcile interval. It stays false for
	// longer only when some node in the fleet is genuinely unreachable,
	// which is the exact condition worth surfacing rather than hiding
	// behind an optimistic "done."
	Confirmed   bool
	ConfirmedAt time.Time
}

// ErrRotationNotSupported is returned by Coordinator.RotateKey when its
// sink does not implement KeyRotator; today that means "this is not the
// control plane's own local node," since LocalSink is the only
// implementation (see this file's own header and ConfigSink's doc
// comment on the gRPC arm not existing yet).
var ErrRotationNotSupported = errors.New("network: this sink does not support key rotation")

// RotateKey rotates nodeID's WireGuard key through the Coordinator's
// sink and begins tracking its confirmation. It does not itself run a
// distribute pass: the caller (the mesh reconcile controller, on its own
// schedule, or an explicit nudge) drives the next Distribute, which is
// what actually carries the new key to every peer. That separation keeps
// this method fast and side-effect-scoped to "this one node's identity
// changed," the same division Distribute itself draws between planning
// and sending.
func (c *Coordinator) RotateKey(ctx context.Context, nodeID string) (RotationResult, error) {
	if err := ctx.Err(); err != nil {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: %w", nodeID, err)
	}
	rotator, ok := c.sink.(KeyRotator)
	if !ok {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: %w", nodeID, ErrRotationNotSupported)
	}

	result, err := rotator.RotateKey(ctx, nodeID)
	if err != nil {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: %w", nodeID, err)
	}

	c.mu.Lock()
	if c.rotations == nil {
		c.rotations = map[string]*RotationStatus{}
	}
	c.rotations[nodeID] = &RotationStatus{
		NodeID:       nodeID,
		OldPublicKey: result.OldPublicKey,
		NewPublicKey: result.NewPublicKey,
		StartedAt:    time.Now(),
	}
	c.mu.Unlock()

	c.logger.Info("rotated node mesh key", slog.String("node_id", nodeID))
	return result, nil
}

// noteDistributeResult updates rotation confirmation bookkeeping after a
// Distribute pass. Called with no lock held; takes it internally.
func (c *Coordinator) noteDistributeResult(result DistributeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.rotations) == 0 {
		return
	}
	failed := len(result.Failed()) > 0
	for _, status := range c.rotations {
		if status.Confirmed || failed {
			continue
		}
		status.Confirmed = true
		status.ConfirmedAt = time.Now()
	}
}

// RotationStatuses returns every rotation this Coordinator has tracked
// since it was constructed, most recently started first. Rotation state
// lives only in memory, the same "not persisted, observedEndpoints has
// the same shape" tradeoff Coordinator already makes for observed
// endpoints: a control plane restart loses the in-flight confirmation
// watch, not the rotation itself (the new key is already live on the
// device and already persisted to the node row by the mesh controller's
// own UpdateNodeMesh call, see internal/reconcile/mesh). What is lost is
// only the operator-facing "still confirming" progress indicator, which
// the next Distribute pass re-establishes as ordinary steady state
// anyway.
func (c *Coordinator) RotationStatuses() []RotationStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]RotationStatus, 0, len(c.rotations))
	for _, s := range c.rotations {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// RotationStatusFor returns the most recent rotation tracked for nodeID,
// if any.
func (c *Coordinator) RotationStatusFor(nodeID string) (RotationStatus, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.rotations[nodeID]
	if !ok {
		return RotationStatus{}, false
	}
	return *s, true
}

// RotateKey generates a fresh private key for this sink's own node, makes
// it live on the mesh immediately (reapplying the last config this sink
// sent, with the new key in place of the old), and returns the identity
// change. nodeID must match this sink's own node, the same restriction
// ApplyMesh already enforces.
//
// "Makes it live immediately" means calling mesh.Apply with the new
// private key before this method returns, rather than waiting for the
// next scheduled ApplyMesh from a Distribute pass: an operator who just
// clicked "rotate key" should not have to wait out an arbitrary reconcile
// interval to see the device actually holding the new key, and Apply is
// cheap and idempotent regardless of who calls it (Mesh's own contract).
//
// If persistFn is set (WithKeyPersistFunc), it is called with the new
// key before the swap and must succeed first: persisting the new key is
// what makes it survive a restart, and applying a key that failed to
// persist would leave the node holding an identity its own key file does
// not agree with, unrecoverable without operator intervention.
func (s *LocalSink) RotateKey(ctx context.Context, nodeID string) (RotationResult, error) {
	if nodeID != s.nodeID {
		return RotationResult{}, fmt.Errorf("%w: local sink is node %q, was asked to rotate %q",
			ErrUnknownNode, s.nodeID, nodeID)
	}

	newPriv, err := GeneratePrivateKey()
	if err != nil {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: generate: %w", nodeID, err)
	}
	newPub, err := newPriv.PublicKey()
	if err != nil {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: derive public key: %w", nodeID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.persistFn != nil {
		if err := s.persistFn(newPriv); err != nil {
			return RotationResult{}, fmt.Errorf("network: rotate key for node %q: persist new key: %w", nodeID, err)
		}
	}

	oldPub := s.publicKey
	cfg := s.lastCfg
	cfg.NodeID = s.nodeID
	cfg.PrivateKey = newPriv

	if err := s.mesh.Apply(ctx, cfg); err != nil {
		return RotationResult{}, fmt.Errorf("network: rotate key for node %q: apply new key: %w", nodeID, err)
	}

	s.privateKey = newPriv
	s.publicKey = newPub
	s.lastCfg = cfg

	return RotationResult{NodeID: nodeID, OldPublicKey: oldPub, NewPublicKey: newPub}, nil
}

// WithKeyPersistFunc sets the callback RotateKey uses to persist a newly
// generated private key before it goes live, so a process restart picks
// up the same key rather than generating yet another one. Callers (the
// binary that owns the key file, e.g. cmd/levelrail's
// loadOrGenerateMeshKey) pass a function that writes the key to disk;
// without one, a rotated key lives only in memory and a restart reverts
// this node to its previous on-disk key while every peer has already
// moved on to the one that no longer persists, which is exactly the
// stuck-rotation state RotationStatus exists to make visible rather than
// prevent outright.
func WithKeyPersistFunc(fn func(Key) error) LocalSinkOption {
	return func(s *LocalSink) { s.persistFn = fn }
}
