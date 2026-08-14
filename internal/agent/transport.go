// Package agent is TASKS.md 3.1's transport boundary: the interface
// ADR 003 describes ("the reconciler and everything above the transport
// boundary never knows whether it's talking to a local in-process agent
// or a remote one over mTLS") but that Phase 1 never actually built,
// confirmed directly against this repo before writing this package:
// every reconcile controller (internal/reconcile/application,
// internal/reconcile/database) takes a bare docker.Runtime, and
// cmd/levelrail/main.go's dynamicSource hands every controller the same
// single local docker.Runtime with no node concept anywhere. There is
// exactly one implicit node today.
//
// This package makes that boundary explicit without moving it: Transport
// has exactly docker.Runtime's method set (see the type below for why),
// so nothing above this package changes shape yet. Wiring
// internal/reconcile controllers to actually route through a Transport
// selected per node, instead of the single shared docker.Runtime
// dynamicSource still hands out today, is TASKS.md 3.3's job, once
// placement (which service runs on which node) exists to select by.
// The real gRPC implementation of Transport, reached over the reverse-
// dialed mTLS connection ADR 003 describes, is TASKS.md 3.2.
package agent

import (
	"errors"
	"fmt"
	"sync"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Transport is what a reconcile controller needs from whichever node a
// resource is placed on. Deliberately identical in shape to
// docker.Runtime (embedded, not duplicated field by field, so the two
// can never silently drift apart) rather than a new, narrower interface:
// docker.Runtime is already the exact narrow surface TASKS.md 1.2/1.3
// scoped down to "what a reconcile controller needs from Docker," and
// Transport's whole point is "the same thing, now reachable on a
// specific node," not a different capability set. A future RPC surface
// an agent exposes beyond container operations (build dispatch for 3.5,
// telemetry collection already covered by ADR 008's separate federated
// design) extends Transport then, not now: this pass only closes the
// gap ADR 003 already described as existing.
type Transport interface {
	docker.Runtime
}

// Local adapts a docker.Runtime this process already has open (this
// machine's own Docker socket, via docker.NewClient) into a Transport
// for this process's own node. This is the node-communication design's
// "single-node mode... in-memory transport that implements the same
// interface the gRPC transport implements," made real: every method
// call happens in-process, no network, no serialization, and (because
// Transport is exactly docker.Runtime's shape) any docker.Runtime
// value already satisfies Transport structurally without needing this
// wrapper type at all. Local exists anyway, as a named, documented
// adapter, so call sites read "this is a node transport" rather than
// relying on Go's structural typing to make that intent legible.
type Local struct {
	docker.Runtime
}

// NewLocal wraps rt as this process's own node Transport.
func NewLocal(rt docker.Runtime) Local {
	return Local{Runtime: rt}
}

// ErrNodeNotRegistered is Registry.Get's failure mode for an unknown
// node ID.
var ErrNodeNotRegistered = errors.New("agent: node not registered in this transport registry")

// Registry resolves a node ID to the Transport that reaches it. A
// single-node deployment registers exactly one entry (its own Local
// transport, dynamicSource's own local docker.Runtime wrapped per
// Local's doc comment); a multi-node deployment (TASKS.md 3.2/3.3)
// registers one entry per enrolled node, most of them real gRPC
// transports. Registry itself has no opinion on how a node's transport
// got built or how long it should live, only "given an ID, hand back
// the Transport for it," the same "pure lookup, not a decision" shape
// GetAPITokenByHash establishes for a different resource in
// internal/store.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]Transport
}

// NewRegistry builds an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Transport)}
}

// Register associates nodeID with t, replacing any existing entry for
// that ID. Safe for concurrent use.
func (r *Registry) Register(nodeID string, t Transport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[nodeID] = t
}

// Unregister removes nodeID's entry, if any. Not an error if nodeID was
// never registered, matching this codebase's established idempotent-
// delete convention (store.DeleteDesiredService, store.DeleteNode).
func (r *Registry) Unregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, nodeID)
}

// Get returns the Transport registered for nodeID, or
// ErrNodeNotRegistered.
func (r *Registry) Get(nodeID string) (Transport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNodeNotRegistered, nodeID)
	}
	return t, nil
}
