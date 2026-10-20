package nodefailure

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditUnreachable = "node.failure.unreachable"
	auditRecovering  = "node.failure.recovering"
	auditRecovered   = "node.failure.recovered"
	auditReschedule  = "node.failure.reschedule"
	auditFence       = "node.failure.fence"
	auditLost        = "node.failure.lost"
	auditResolve     = "node.failure.resolve"
)

func (c *Controller) process(ctx context.Context, n store.Node) error {
	mode, grace, err := c.policy(ctx, n.ID)
	if err != nil {
		return err
	}
	st, err := c.store.GetNodeFailureState(ctx, n.ID)
	if err != nil {
		return err
	}
	if !c.unreachable(n, grace) {
		if st == nil {
			return nil
		}
		return c.healthy(ctx, n, st)
	}
	if st == nil {
		if mode == store.NodeFailureModeOff {
			return nil
		}
		since := c.now()
		if n.LastSeenAt != nil {
			since = *n.LastSeenAt
		}
		st = &store.NodeFailureState{NodeID: n.ID, Phase: store.NodeFailurePhaseUnreachable, UnreachableAt: since,
			Reason: fmt.Sprintf("no heartbeat for over %s", grace)}
		c.exec.Audit(ctx, auditUnreachable, fmt.Sprintf("node %s mode=%s", n.ID, mode))
	} else if st.Phase == store.NodeFailurePhaseRecovering {
		st.Phase, st.RecoveringAt = store.NodeFailurePhaseUnreachable, nil
		c.exec.Audit(ctx, auditUnreachable, fmt.Sprintf("node %s dropped again while recovering", n.ID))
	}
	return c.advance(ctx, n, st, mode == store.NodeFailureModeAuto || st.Phase == store.NodeFailurePhaseLost)
}

// advance merges what still runs on the node into the action list and, when act is set, moves stateless apps.
func (c *Controller) advance(ctx context.Context, n store.Node, st *store.NodeFailureState, act bool) error {
	resources, err := c.exec.Resources(ctx, n.ID)
	if err != nil {
		return fmt.Errorf("list resources on %s: %w", n.ID, err)
	}
	c.merge(st, resources)
	if act {
		c.cordon(ctx, n, st)
		for i := range st.Actions {
			a := &st.Actions[i]
			if a.Class != store.NodeFailureClassReschedule || a.Status == store.NodeFailureActionMoved || a.Status == store.NodeFailureActionFenced {
				continue
			}
			c.reschedule(ctx, n.ID, st, a, resources)
		}
	}
	return c.store.SaveNodeFailureState(ctx, *st)
}

func (c *Controller) merge(st *store.NodeFailureState, resources []Resource) {
	on := map[string]bool{}
	for _, r := range resources {
		on[r.Kind+"/"+r.Name] = true
	}
	kept := st.Actions[:0]
	for _, a := range st.Actions {
		done := a.Status == store.NodeFailureActionMoved || a.Status == store.NodeFailureActionFenced
		if done || on[a.Kind+"/"+a.Name] {
			kept = append(kept, a)
		}
	}
	st.Actions = kept
	have := map[string]bool{}
	for _, a := range st.Actions {
		have[a.Kind+"/"+a.Name] = true
	}
	for _, r := range resources {
		if have[r.Kind+"/"+r.Name] {
			continue
		}
		class, reason := Classify(r)
		st.Actions = append(st.Actions, store.NodeFailureAction{Kind: r.Kind, Name: r.Name, Class: class, Status: store.NodeFailureActionPending, Reason: reason})
	}
}

func (c *Controller) cordon(ctx context.Context, n store.Node, st *store.NodeFailureState) {
	if !n.Schedulable || st.CordonedByUs {
		return
	}
	if err := c.store.SetNodeSchedulable(ctx, n.ID, false); err != nil {
		c.logger.Warn("nodefailure: cordon failed", slog.String("node_id", n.ID), slog.String("error", err.Error()))
		return
	}
	st.CordonedByUs = true
}

func (c *Controller) reschedule(ctx context.Context, nodeID string, st *store.NodeFailureState, a *store.NodeFailureAction, resources []Resource) {
	cur, err := c.exec.NodeOf(ctx, a.Kind, a.Name)
	if err == nil && cur != "" && cur != nodeID {
		a.Status = store.NodeFailureActionMoved
		return
	}
	res := Resource{Kind: a.Kind, Name: a.Name}
	for _, r := range resources {
		if r.Kind == a.Kind && r.Name == a.Name {
			res = r
		}
	}
	target := a.Target
	if a.Status != store.NodeFailureActionMoving || target == "" {
		target, err = c.exec.PickTarget(ctx, res, nodeID, true)
		if err != nil {
			a.Status, a.Reason = store.NodeFailureActionFailed, "no healthy node can take it: "+err.Error()
			return
		}
	}
	a.Status, a.Target = store.NodeFailureActionMoving, target
	if err := c.store.SaveNodeFailureState(ctx, *st); err != nil {
		a.Status, a.Reason = store.NodeFailureActionFailed, err.Error()
		return
	}
	if err := c.exec.Place(ctx, a.Kind, a.Name, target); err != nil {
		a.Status, a.Reason = store.NodeFailureActionFailed, "placing failed: "+err.Error()
		c.exec.Audit(ctx, auditReschedule, fmt.Sprintf("%s %s from %s to %s failed: %s", a.Kind, a.Name, nodeID, target, err.Error()))
		return
	}
	a.Status, a.Reason = store.NodeFailureActionMoved, "moved to "+target
	c.exec.Audit(ctx, auditReschedule, fmt.Sprintf("%s %s from %s to %s", a.Kind, a.Name, nodeID, target))
}

// healthy fences stale copies right away, then clears the state after the recovery hold.
func (c *Controller) healthy(ctx context.Context, n store.Node, st *store.NodeFailureState) error {
	allFenced := true
	for i := range st.Actions {
		a := &st.Actions[i]
		if a.Status != store.NodeFailureActionMoved {
			continue
		}
		cur, err := c.exec.NodeOf(ctx, a.Kind, a.Name)
		if err == nil && cur == n.ID {
			a.Status, a.Reason = store.NodeFailureActionFenced, "placed back on this node by an operator"
			continue
		}
		if err := c.exec.Fence(ctx, a.Kind, a.Name, n.ID); err != nil {
			a.Reason = "fencing failed, retrying: " + err.Error()
			allFenced = false
			continue
		}
		a.Status, a.Reason = store.NodeFailureActionFenced, "stale copy stopped on the returned node"
		c.exec.Audit(ctx, auditFence, fmt.Sprintf("%s %s stopped on %s", a.Kind, a.Name, n.ID))
	}
	now := c.now()
	if st.Phase != store.NodeFailurePhaseRecovering {
		st.Phase, st.RecoveringAt = store.NodeFailurePhaseRecovering, &now
		c.exec.Audit(ctx, auditRecovering, "node "+n.ID)
	}
	if allFenced && now.Sub(*st.RecoveringAt) >= c.cfg.RecoveryHold {
		return c.clear(ctx, n, st)
	}
	return c.store.SaveNodeFailureState(ctx, *st)
}

func (c *Controller) clear(ctx context.Context, n store.Node, st *store.NodeFailureState) error {
	if st.CordonedByUs {
		if err := c.store.SetNodeSchedulable(ctx, n.ID, true); err != nil {
			return fmt.Errorf("uncordon %s: %w", n.ID, err)
		}
	}
	if err := c.store.DeleteNodeFailureState(ctx, n.ID); err != nil {
		return err
	}
	c.exec.Audit(ctx, auditRecovered, "node "+n.ID)
	return nil
}
