package nodemove

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Resource is one app or database a drain must move off a node.
type Resource struct {
	Kind    string
	Name    string
	Volumes []string
}

// BlockedError means a resource cannot be moved by a drain at all.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return e.Reason }

// DrainPlanner supplies what a drain needs from the platform.
type DrainPlanner interface {
	Cordon(ctx context.Context, nodeID string) error
	// Remaining lists what still runs on nodeID, databases before apps so
	// an app can follow its database.
	Remaining(ctx context.Context, nodeID string) ([]Resource, error)
	// PickTarget chooses where res goes, honoring explicit when set. A
	// *BlockedError skips res and records why.
	PickTarget(ctx context.Context, res Resource, fromNodeID, explicit string) (string, error)
	// Blocked lists resources on nodeID a drain can never move (e.g. models).
	Blocked(ctx context.Context, nodeID string) ([]store.NodeDrainBlocked, error)
}

// StartDrain cordons nodeID and records a running drain. It fails with
// store.ErrNodeMoveInProgress when the node is already draining.
func (r *Runner) StartDrain(ctx context.Context, nodeID, explicitTarget string) (store.NodeDrain, error) {
	if r.drains == nil {
		return store.NodeDrain{}, errors.New("nodemove: drains are not configured")
	}
	if err := r.drains.Cordon(ctx, nodeID); err != nil {
		return store.NodeDrain{}, fmt.Errorf("nodemove: cordon %s: %w", nodeID, err)
	}
	id, err := r.newID("dr")
	if err != nil {
		return store.NodeDrain{}, err
	}
	now := r.stamp()
	d := store.NodeDrain{ID: id, NodeID: nodeID, TargetNodeID: explicitTarget, Status: store.NodeMoveStatusRunning, StartedAt: now, UpdatedAt: now}
	if err := r.store.CreateNodeDrain(ctx, d); err != nil {
		return store.NodeDrain{}, err
	}
	r.Kick()
	return d, nil
}

// AdvanceDrain moves the next resource off the drained node, one at a
// time, each through a full verified move. It stops at the first failed
// move and leaves the node cordoned.
func (r *Runner) AdvanceDrain(ctx context.Context, id string) error {
	d, err := r.store.GetNodeDrain(ctx, id)
	if err != nil {
		return fmt.Errorf("nodemove: load drain %s: %w", id, err)
	}
	if d.Status != store.NodeMoveStatusRunning {
		return nil
	}
	children, err := r.store.ListNodeMoves(ctx, store.NodeMoveFilter{DrainID: id})
	if err != nil {
		return fmt.Errorf("nodemove: list moves of drain %s: %w", id, err)
	}
	for _, c := range children {
		switch c.Status {
		case store.NodeMoveStatusRunning:
			return nil
		case store.NodeMoveStatusFailed, store.NodeMoveStatusRolledBack:
			return r.finishDrain(ctx, &d, store.NodeMoveStatusFailed, fmt.Sprintf("moving %s %s failed: %s", c.Kind, c.Resource, c.Error))
		}
	}

	remaining, err := r.drains.Remaining(ctx, d.NodeID)
	if err != nil {
		return fmt.Errorf("nodemove: list resources on %s: %w", d.NodeID, err)
	}
	skip := map[string]bool{}
	for _, b := range d.Blocked {
		skip[b.Kind+"/"+b.Name] = true
	}
	for _, res := range remaining {
		if skip[res.Kind+"/"+res.Name] {
			continue
		}
		target, err := r.drains.PickTarget(ctx, res, d.NodeID, d.TargetNodeID)
		var blocked *BlockedError
		if errors.As(err, &blocked) {
			d.Blocked = append(d.Blocked, store.NodeDrainBlocked{Kind: res.Kind, Name: res.Name, Reason: blocked.Reason})
			skip[res.Kind+"/"+res.Name] = true
			continue
		}
		if err != nil {
			return fmt.Errorf("nodemove: pick target for %s %s: %w", res.Kind, res.Name, err)
		}
		if _, err := r.Start(ctx, Request{Kind: res.Kind, Resource: res.Name, FromNodeID: d.NodeID, ToNodeID: target, Volumes: res.Volumes, DrainID: d.ID}); err != nil {
			if errors.Is(err, store.ErrNodeMoveInProgress) {
				// Moved by hand right now; pick it up again on a later tick.
				return r.saveDrain(ctx, &d)
			}
			return r.finishDrain(ctx, &d, store.NodeMoveStatusFailed, fmt.Sprintf("starting move of %s %s: %s", res.Kind, res.Name, err.Error()))
		}
		return r.saveDrain(ctx, &d)
	}

	extra, err := r.drains.Blocked(ctx, d.NodeID)
	if err != nil {
		return fmt.Errorf("nodemove: list blocked resources on %s: %w", d.NodeID, err)
	}
	for _, b := range extra {
		if !skip[b.Kind+"/"+b.Name] {
			d.Blocked = append(d.Blocked, b)
		}
	}
	if len(d.Blocked) > 0 {
		names := make([]string, 0, len(d.Blocked))
		for _, b := range d.Blocked {
			names = append(names, b.Kind+" "+b.Name)
		}
		return r.finishDrain(ctx, &d, store.NodeMoveStatusFailed, "could not move: "+strings.Join(names, ", "))
	}
	return r.finishDrain(ctx, &d, store.NodeMoveStatusSucceeded, "")
}

func (r *Runner) finishDrain(ctx context.Context, d *store.NodeDrain, status, msg string) error {
	d.Status, d.Error, d.FinishedAt = status, msg, r.stamp()
	return r.saveDrain(ctx, d)
}

func (r *Runner) saveDrain(ctx context.Context, d *store.NodeDrain) error {
	d.UpdatedAt = r.stamp()
	if err := r.store.SaveNodeDrain(ctx, *d); err != nil {
		return fmt.Errorf("nodemove: save drain %s: %w", d.ID, err)
	}
	return nil
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("nodemove: generate id: %w", err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
