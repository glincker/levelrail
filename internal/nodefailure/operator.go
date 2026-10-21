package nodefailure

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Resolution actions an operator can choose for a resource that needs a decision.
const (
	ResolveWait    = "wait"
	ResolveRestore = "restore"
)

// Status is what the dashboard and CLI show for one node.
type Status struct {
	NodeID string `json:"node_id"`
	// Mode and GraceSeconds are the effective policy (override or default).
	Mode         string `json:"mode"`
	GraceSeconds int    `json:"grace_seconds"`
	// Phase is "", "grace" (silent but inside the grace period), or a store.NodeFailurePhase*.
	Phase string `json:"phase"`
	// SilentSeconds and GraceRemainingSeconds drive the grace timer.
	SilentSeconds         int                       `json:"silent_seconds"`
	GraceRemainingSeconds int                       `json:"grace_remaining_seconds"`
	Reason                string                    `json:"reason,omitempty"`
	UnreachableAt         *time.Time                `json:"unreachable_at,omitempty"`
	Consequences          []store.NodeFailureAction `json:"consequences"`
	Policy                store.NodeFailurePolicy   `json:"policy"`
	State                 *store.NodeFailureState   `json:"-"`
}

// Status reports a node's failure state and, while it is unreachable, the
// per-resource consequences (with would-be targets when nothing has run yet).
func (c *Controller) Status(ctx context.Context, nodeID string) (Status, error) {
	n, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	return c.statusFor(ctx, *n)
}

func (c *Controller) statusFor(ctx context.Context, n store.Node) (Status, error) {
	mode, grace, err := c.policy(ctx, n.ID)
	if err != nil {
		return Status{}, err
	}
	pol, err := c.store.GetNodeFailurePolicy(ctx, n.ID)
	if err != nil {
		return Status{}, err
	}
	st, err := c.store.GetNodeFailureState(ctx, n.ID)
	if err != nil {
		return Status{}, err
	}
	out := Status{NodeID: n.ID, Mode: mode, GraceSeconds: int(grace.Seconds()), Consequences: []store.NodeFailureAction{}, Policy: pol, State: st}
	silent, seen := silentFor(n, c.now())
	if seen {
		out.SilentSeconds = int(silent.Seconds())
	}
	local := c.exec.IsLocal(n.ID) || n.Status == store.NodeStatusPending
	down := !local && c.unreachable(n, grace)
	switch {
	case st != nil:
		out.Phase, out.Reason, out.Consequences = st.Phase, st.Reason, st.Actions
		t := st.UnreachableAt
		out.UnreachableAt = &t
	case down:
		out.Phase = store.NodeFailurePhaseUnreachable
		out.Reason = "no heartbeat, failure policy is " + mode
	case !local && n.Status == store.NodeStatusOffline:
		out.Phase = "grace"
		if rem := grace - silent; rem > 0 {
			out.GraceRemainingSeconds = int(rem.Seconds())
		}
	}
	if st == nil && out.Phase != "" {
		if out.Consequences, err = c.preview(ctx, n.ID); err != nil {
			return Status{}, err
		}
	}
	return out, nil
}

// preview is the plan that would run if the node failed now; it changes nothing.
func (c *Controller) preview(ctx context.Context, nodeID string) ([]store.NodeFailureAction, error) {
	resources, err := c.exec.Resources(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list resources on %s: %w", nodeID, err)
	}
	out := make([]store.NodeFailureAction, 0, len(resources))
	for _, r := range resources {
		class, reason := Classify(r)
		a := store.NodeFailureAction{Kind: r.Kind, Name: r.Name, Class: class, Status: store.NodeFailureActionPending, Reason: reason}
		if class == store.NodeFailureClassReschedule {
			if target, err := c.exec.PickTarget(ctx, r, nodeID, false); err != nil {
				a.Reason = "no healthy node can take it: " + err.Error()
			} else {
				a.Target = target
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// MarkLost declares an unreachable node lost: stateless apps move now and the
// rest wait for a per-resource decision. With dryRun it only returns the plan.
func (c *Controller) MarkLost(ctx context.Context, nodeID string, dryRun bool) (Status, error) {
	n, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	_, grace, err := c.policy(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	if !c.unreachable(*n, grace) && n.Status != store.NodeStatusOffline {
		return Status{}, ErrNodeHealthy
	}
	if dryRun {
		plan, err := c.preview(ctx, nodeID)
		if err != nil {
			return Status{}, err
		}
		s, err := c.statusFor(ctx, *n)
		s.Consequences = plan
		return s, err
	}
	st, err := c.store.GetNodeFailureState(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	if st == nil {
		since := c.now()
		if n.LastSeenAt != nil {
			since = *n.LastSeenAt
		}
		st = &store.NodeFailureState{NodeID: nodeID, UnreachableAt: since}
	}
	st.Phase, st.RecoveringAt, st.Reason = store.NodeFailurePhaseLost, nil, "marked lost by an operator"
	c.exec.Audit(ctx, auditLost, "node "+nodeID)
	if err := c.advance(ctx, *n, st, true); err != nil {
		return Status{}, err
	}
	return c.statusFor(ctx, *n)
}

// Resolve records an operator's decision for one resource of a failed node.
// ResolveRestore places it on target (or a picked node) so its data can be
// restored from backup there; the old copy is fenced when the node returns.
func (c *Controller) Resolve(ctx context.Context, nodeID, kind, name, action, target string) (Status, error) {
	n, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	st, err := c.store.GetNodeFailureState(ctx, nodeID)
	if err != nil {
		return Status{}, err
	}
	if st == nil || st.Phase == store.NodeFailurePhaseRecovering {
		return Status{}, ErrNoFailure
	}
	var a *store.NodeFailureAction
	for i := range st.Actions {
		if st.Actions[i].Kind == kind && st.Actions[i].Name == name {
			a = &st.Actions[i]
		}
	}
	if a == nil {
		return Status{}, ErrNoSuchAction
	}
	switch action {
	case ResolveWait:
		a.Status, a.Reason = store.NodeFailureActionWaiting, "operator chose to wait for the node to return"
	case ResolveRestore:
		if target == "" {
			if target, err = c.exec.PickTarget(ctx, Resource{Kind: kind, Name: name}, nodeID, true); err != nil {
				return Status{}, fmt.Errorf("pick target: %w", err)
			}
		}
		if err := c.exec.Place(ctx, kind, name, target); err != nil {
			return Status{}, fmt.Errorf("place %s %s on %s: %w", kind, name, target, err)
		}
		a.Status, a.Target, a.Reason = store.NodeFailureActionMoved, target, "placed on "+target+" for restore from backup"
	default:
		return Status{}, fmt.Errorf("nodefailure: unknown action %q", action)
	}
	c.exec.Audit(ctx, auditResolve, fmt.Sprintf("%s %s %s on node %s target=%s", action, kind, name, nodeID, target))
	if err := c.store.SaveNodeFailureState(ctx, *st); err != nil {
		return Status{}, err
	}
	return c.statusFor(ctx, *n)
}

// SetPolicy stores a node's policy override; an empty mode and zero grace restore the defaults.
func (c *Controller) SetPolicy(ctx context.Context, p store.NodeFailurePolicy) error {
	return c.store.SetNodeFailurePolicy(ctx, p)
}

// States returns every live failure state keyed by node ID.
func (c *Controller) States(ctx context.Context) (map[string]store.NodeFailureState, error) {
	return c.store.ListNodeFailureStates(ctx)
}
