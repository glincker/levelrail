// Package nodehealth implements TASKS.md 3.7's node health check: the
// reconcile.Controller that converges a node's observed heartbeat
// (internal/store's last_seen_at, kept fresh by internal/agent.Server's
// periodic touch loop while a node's gRPC session stays open) against
// its recorded Status, the same architectural pattern
// internal/reconcile/application and internal/reconcile/database already
// establish (level-triggered, a narrow store interface for testability,
// one controller instance per resource), applied to a node instead of a
// service or database.
//
// This exists specifically because internal/agent.Server.Session already
// flips a node's Status to Offline synchronously when its gRPC stream
// ends (a graceful disconnect), but a stream that never ends cleanly
// (a hard process kill, a severed network link with no TCP FIN) leaves
// Status stuck at Online forever with nothing to notice otherwise. This
// controller is that "otherwise": every reconcile pass re-reads
// LastSeenAt fresh from the store (never cached) and compares it against
// a timeout, the same level-triggered principle required of every
// controller in this codebase, so a hung connection is
// discovered on the very next resync tick rather than staying silently
// wrong until someone happens to look.
package nodehealth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ConditionType is the Type of the single condition this controller
// reports. Exported so internal/api's handleGetNodeHealth (and any
// future caller reading internal/store.GetConditions results) can name
// it without duplicating the string.
const ConditionType = "Heartbeat"

// Store is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database. *store.DB
// satisfies this structurally.
type Store interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)
	UpdateNodeStatus(ctx context.Context, id string, status store.NodeStatus) error
}

// Controller converges one named node's observed heartbeat (read fresh
// from Store on every Reconcile, never cached) against its Status.
type Controller struct {
	nodeID  string
	store   Store
	timeout time.Duration
	now     func() time.Time
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithClock overrides the controller's notion of "now", for
// deterministic tests. Without one configured, time.Now is used.
func WithClock(now func() time.Time) Option {
	return func(c *Controller) { c.now = now }
}

// New builds a Controller for nodeID. timeout is how long since
// LastSeenAt is treated as stale; callers read it from an env var, per
// the house rule against hardcoded thresholds, and this package
// itself has no opinion on a default.
func New(nodeID string, st Store, timeout time.Duration, opts ...Option) *Controller {
	c := &Controller{nodeID: nodeID, store: st, timeout: timeout, now: time.Now}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "node-health/" + c.nodeID }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	node, err := c.store.GetNode(ctx, c.nodeID)
	if errors.Is(err, store.ErrNodeNotFound) {
		// Not a failure: this controller can be built for a node that
		// was deleted between the dynamicSource listing that produced
		// it and this call actually running, the same "not found is a
		// legitimate transient state, not an error" precedent
		// database.Controller's own NoDesiredState result already sets
		// for a deleted database.
		return unknownResult("NodeNotFound", "node no longer exists"), nil
	}
	if err != nil {
		return unknownResult("StoreError", err.Error()), fmt.Errorf("node-health/%s: get node: %w", c.nodeID, err)
	}

	if node.Status == store.NodeStatusPending {
		return unknownResult("NeverConnected", "node enrolled but has not connected yet"), nil
	}

	now := c.now()
	since, everSeen := lastSeenAge(node, now)
	if !everSeen || since > c.timeout {
		msg := staleMessage(c.timeout, everSeen, since)

		// Only actually flip Status when it's still Online: an already-
		// Offline node (e.g. one that disconnected gracefully, which
		// Server.Session's own defer already marked Offline) has
		// nothing left to converge here, matching every other
		// controller's "cheap no-op when nothing changed" idempotency
		// requirement (the reconciler contract, this package's own doc comment).
		if node.Status == store.NodeStatusOnline {
			if err := c.store.UpdateNodeStatus(ctx, c.nodeID, store.NodeStatusOffline); err != nil {
				return unknownResult("MarkOfflineFailed", err.Error()),
					fmt.Errorf("node-health/%s: mark offline: %w", c.nodeID, err)
			}
		}
		return falseResult("HeartbeatStale", msg), nil
	}

	return trueResult("HeartbeatRecent", fmt.Sprintf("heartbeat received %s ago", since.Round(time.Second))), nil
}

// lastSeenAge returns how long ago node.LastSeenAt was, and whether it
// was ever set at all (a node can be Online in Status only through
// Server.Session, which always touches LastSeenAt before registering
// the connection, so a nil LastSeenAt on a non-Pending node is not
// expected in practice, but is handled as "stale" rather than assumed
// impossible, the same defensive posture notReady's own callers already
// take toward unexpected states elsewhere in this codebase).
func lastSeenAge(node *store.Node, now time.Time) (age time.Duration, everSeen bool) {
	if node.LastSeenAt == nil {
		return 0, false
	}
	return now.Sub(*node.LastSeenAt), true
}

func staleMessage(timeout time.Duration, everSeen bool, since time.Duration) string {
	if !everSeen {
		return fmt.Sprintf("no heartbeat ever recorded (timeout %s)", timeout)
	}
	return fmt.Sprintf("no heartbeat received for %s, over the %s timeout", since.Round(time.Second), timeout)
}

func trueResult(reason, message string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: ConditionType, Status: reconcile.ConditionTrue, Reason: reason, Message: message,
	}}}
}

func falseResult(reason, message string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: ConditionType, Status: reconcile.ConditionFalse, Reason: reason, Message: message,
	}}}
}

func unknownResult(reason, message string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: ConditionType, Status: reconcile.ConditionUnknown, Reason: reason, Message: message,
	}}}
}
