package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Node failure modes. The empty mode on a per-node policy means "use the control plane default".
const (
	NodeFailureModeOff    = "off"
	NodeFailureModeNotify = "notify"
	NodeFailureModeAuto   = "auto"
)

// Node failure phases.
const (
	NodeFailurePhaseUnreachable = "unreachable"
	NodeFailurePhaseRecovering  = "recovering"
	NodeFailurePhaseLost        = "lost"
)

// Per-resource consequence classes and statuses recorded in a failure state.
const (
	NodeFailureClassReschedule = "reschedule"
	NodeFailureClassDecision   = "needs_decision"

	NodeFailureActionPending = "pending"
	NodeFailureActionMoving  = "moving"
	NodeFailureActionMoved   = "moved"
	NodeFailureActionFenced  = "fenced"
	NodeFailureActionWaiting = "waiting"
	NodeFailureActionFailed  = "failed"
)

// IsNodeFailureMode reports whether m is a mode a policy may hold.
func IsNodeFailureMode(m string) bool {
	return m == NodeFailureModeOff || m == NodeFailureModeNotify || m == NodeFailureModeAuto
}

// NodeFailurePolicy is one node's override of the failure policy.
type NodeFailurePolicy struct {
	NodeID       string `json:"node_id"`
	Mode         string `json:"mode"`
	GraceSeconds int    `json:"grace_seconds"`
}

// NodeFailureAction is what happened, or must happen, to one resource of a failed node.
type NodeFailureAction struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Class  string `json:"class"`
	Status string `json:"status"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NodeFailureState is the live failure record of one node; no row means the node is healthy.
type NodeFailureState struct {
	NodeID        string              `json:"node_id"`
	Phase         string              `json:"phase"`
	Reason        string              `json:"reason"`
	UnreachableAt time.Time           `json:"unreachable_at"`
	RecoveringAt  *time.Time          `json:"recovering_at,omitempty"`
	CordonedByUs  bool                `json:"cordoned_by_us"`
	Actions       []NodeFailureAction `json:"actions"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// GetNodeFailurePolicy returns a node's policy override, zero-valued when none is set.
func (db *DB) GetNodeFailurePolicy(ctx context.Context, nodeID string) (NodeFailurePolicy, error) {
	p := NodeFailurePolicy{NodeID: nodeID}
	err := db.QueryRowContext(ctx, `SELECT mode, grace_seconds FROM node_failure_policy WHERE node_id = ?`, nodeID).Scan(&p.Mode, &p.GraceSeconds)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("store: get node failure policy %q: %w", nodeID, err)
	}
	return p, nil
}

// ListNodeFailurePolicies returns every stored override keyed by node ID.
func (db *DB) ListNodeFailurePolicies(ctx context.Context) (map[string]NodeFailurePolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id, mode, grace_seconds FROM node_failure_policy`)
	if err != nil {
		return nil, fmt.Errorf("store: list node failure policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]NodeFailurePolicy{}
	for rows.Next() {
		var p NodeFailurePolicy
		if err := rows.Scan(&p.NodeID, &p.Mode, &p.GraceSeconds); err != nil {
			return nil, fmt.Errorf("store: scan node failure policy: %w", err)
		}
		out[p.NodeID] = p
	}
	return out, rows.Err()
}

// SetNodeFailurePolicy stores a node's policy override.
func (db *DB) SetNodeFailurePolicy(ctx context.Context, p NodeFailurePolicy) error {
	if _, err := db.GetNode(ctx, p.NodeID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_failure_policy (node_id, mode, grace_seconds, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET mode = excluded.mode, grace_seconds = excluded.grace_seconds, updated_at = excluded.updated_at
	`, p.NodeID, p.Mode, p.GraceSeconds, FormatAuditTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set node failure policy %q: %w", p.NodeID, err)
	}
	return nil
}

// GetNodeFailureState returns a node's failure state, or nil when it has none.
func (db *DB) GetNodeFailureState(ctx context.Context, nodeID string) (*NodeFailureState, error) {
	row := db.QueryRowContext(ctx, failureStateSelect+` WHERE node_id = ?`, nodeID)
	s, err := scanNodeFailureState(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get node failure state %q: %w", nodeID, err)
	}
	return &s, nil
}

// ListNodeFailureStates returns every live failure state keyed by node ID.
func (db *DB) ListNodeFailureStates(ctx context.Context) (map[string]NodeFailureState, error) {
	rows, err := db.QueryContext(ctx, failureStateSelect)
	if err != nil {
		return nil, fmt.Errorf("store: list node failure states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]NodeFailureState{}
	for rows.Next() {
		s, err := scanNodeFailureState(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node failure state: %w", err)
		}
		out[s.NodeID] = s
	}
	return out, rows.Err()
}

// SaveNodeFailureState inserts or replaces a node's failure state.
func (db *DB) SaveNodeFailureState(ctx context.Context, s NodeFailureState) error {
	actions := s.Actions
	if actions == nil {
		actions = []NodeFailureAction{}
	}
	raw, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("store: encode node failure actions: %w", err)
	}
	recovering := ""
	if s.RecoveringAt != nil {
		recovering = FormatAuditTime(*s.RecoveringAt)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_failure_state (node_id, phase, reason, unreachable_at, recovering_at, cordoned_by_us, actions_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET phase = excluded.phase, reason = excluded.reason, unreachable_at = excluded.unreachable_at,
			recovering_at = excluded.recovering_at, cordoned_by_us = excluded.cordoned_by_us, actions_json = excluded.actions_json, updated_at = excluded.updated_at
	`, s.NodeID, s.Phase, s.Reason, FormatAuditTime(s.UnreachableAt), recovering, boolToInt(s.CordonedByUs), string(raw), FormatAuditTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save node failure state %q: %w", s.NodeID, err)
	}
	return nil
}

// DeleteNodeFailureState clears a node's failure state.
func (db *DB) DeleteNodeFailureState(ctx context.Context, nodeID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM node_failure_state WHERE node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("store: delete node failure state %q: %w", nodeID, err)
	}
	return nil
}

const failureStateSelect = `SELECT node_id, phase, reason, unreachable_at, recovering_at, cordoned_by_us, actions_json, updated_at FROM node_failure_state`

func scanNodeFailureState(scan func(dest ...any) error) (NodeFailureState, error) {
	var s NodeFailureState
	var unreachable, recovering, updated, raw string
	var cordoned int
	if err := scan(&s.NodeID, &s.Phase, &s.Reason, &unreachable, &recovering, &cordoned, &raw, &updated); err != nil {
		return s, err
	}
	s.CordonedByUs = cordoned != 0
	s.UnreachableAt, _ = time.Parse(time.RFC3339Nano, unreachable)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if recovering != "" {
		if t, err := time.Parse(time.RFC3339Nano, recovering); err == nil {
			s.RecoveringAt = &t
		}
	}
	if err := json.Unmarshal([]byte(raw), &s.Actions); err != nil {
		return s, fmt.Errorf("decode actions: %w", err)
	}
	if s.Actions == nil {
		s.Actions = []NodeFailureAction{}
	}
	return s, nil
}
