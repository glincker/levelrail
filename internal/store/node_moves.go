package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNodeMoveNotFound is returned when a node move or drain ID matches no row.
var ErrNodeMoveNotFound = errors.New("store: node move not found")

// ErrNodeMoveInProgress is returned when a resource or node already has a running move or drain.
var ErrNodeMoveInProgress = errors.New("store: a move is already in progress for this resource")

// Node move and drain kinds and statuses.
const (
	NodeMoveKindApp      = "app"
	NodeMoveKindDatabase = "database"

	NodeMoveStatusRunning    = "running"
	NodeMoveStatusSucceeded  = "succeeded"
	NodeMoveStatusFailed     = "failed"
	NodeMoveStatusRolledBack = "rolled_back"
)

// NodeMoveStep is one recorded phase transition of a node move.
type NodeMoveStep struct {
	Phase   string `json:"phase"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

// NodeMove is one persisted move of an app or database between nodes.
type NodeMove struct {
	ID             string
	Kind           string
	Resource       string
	FromNodeID     string
	ToNodeID       string
	Stateful       bool
	Volumes        []string
	Phase          string
	Status         string
	Error          string
	Attempts       int
	Steps          []NodeMoveStep
	DrainID        string
	PlacedAt       string
	VerifyDeadline string
	StartedAt      string
	UpdatedAt      string
	FinishedAt     string
}

const nodeMoveColumns = `SELECT id, kind, resource, from_node_id, to_node_id, stateful, volumes, phase, status, error,
	attempts, steps, drain_id, placed_at, verify_deadline, started_at, updated_at, finished_at FROM node_moves`

// CreateNodeMove inserts a running move, failing with ErrNodeMoveInProgress
// when the resource already has one.
func (db *DB) CreateNodeMove(ctx context.Context, m NodeMove) error {
	volumes, steps, err := marshalNodeMoveJSON(m)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_moves (id, kind, resource, from_node_id, to_node_id, stateful, volumes, phase, status, error,
			attempts, steps, drain_id, placed_at, verify_deadline, started_at, updated_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', '', 0, ?, ?, '', '', ?, ?, '')
	`, m.ID, m.Kind, m.Resource, m.FromNodeID, m.ToNodeID, boolToInt(m.Stateful), volumes, m.Phase, steps, m.DrainID, m.StartedAt, m.StartedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNodeMoveInProgress
		}
		return fmt.Errorf("store: create node move %q: %w", m.ID, err)
	}
	return nil
}

// SaveNodeMove persists every mutable field of an existing move.
func (db *DB) SaveNodeMove(ctx context.Context, m NodeMove) error {
	volumes, steps, err := marshalNodeMoveJSON(m)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE node_moves SET phase = ?, status = ?, error = ?, attempts = ?, steps = ?, volumes = ?, placed_at = ?,
			verify_deadline = ?, updated_at = ?, finished_at = ?
		WHERE id = ?
	`, m.Phase, m.Status, m.Error, m.Attempts, steps, volumes, m.PlacedAt, m.VerifyDeadline, m.UpdatedAt, m.FinishedAt, m.ID)
	if err != nil {
		return fmt.Errorf("store: save node move %q: %w", m.ID, err)
	}
	return requireOneRow(res, ErrNodeMoveNotFound, "save node move "+m.ID)
}

// GetNodeMove returns one move by ID.
func (db *DB) GetNodeMove(ctx context.Context, id string) (NodeMove, error) {
	m, err := scanNodeMove(db.QueryRowContext(ctx, nodeMoveColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeMove{}, ErrNodeMoveNotFound
	}
	if err != nil {
		return NodeMove{}, fmt.Errorf("store: get node move %q: %w", id, err)
	}
	return m, nil
}

// NodeMoveFilter narrows ListNodeMoves; empty fields match everything.
type NodeMoveFilter struct {
	Kind     string
	Resource string
	DrainID  string
	Status   string
	Limit    int
}

// ListNodeMoves returns moves newest first.
func (db *DB) ListNodeMoves(ctx context.Context, f NodeMoveFilter) ([]NodeMove, error) {
	var where []string
	var args []any
	for _, c := range []struct{ col, val string }{{"kind", f.Kind}, {"resource", f.Resource}, {"drain_id", f.DrainID}, {"status", f.Status}} {
		if c.val != "" {
			where = append(where, c.col+" = ?")
			args = append(args, c.val)
		}
	}
	q := nodeMoveColumns
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY started_at DESC, id DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list node moves: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeMove
	for rows.Next() {
		m, err := scanNodeMove(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node move: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanNodeMove(scan func(dest ...any) error) (NodeMove, error) {
	var (
		m              NodeMove
		stateful       int
		volumes, steps string
	)
	if err := scan(&m.ID, &m.Kind, &m.Resource, &m.FromNodeID, &m.ToNodeID, &stateful, &volumes, &m.Phase, &m.Status, &m.Error,
		&m.Attempts, &steps, &m.DrainID, &m.PlacedAt, &m.VerifyDeadline, &m.StartedAt, &m.UpdatedAt, &m.FinishedAt); err != nil {
		return NodeMove{}, err
	}
	m.Stateful = stateful != 0
	if err := json.Unmarshal([]byte(volumes), &m.Volumes); err != nil {
		return NodeMove{}, fmt.Errorf("parse volumes: %w", err)
	}
	if err := json.Unmarshal([]byte(steps), &m.Steps); err != nil {
		return NodeMove{}, fmt.Errorf("parse steps: %w", err)
	}
	return m, nil
}

func marshalNodeMoveJSON(m NodeMove) (string, string, error) {
	vols := m.Volumes
	if vols == nil {
		vols = []string{}
	}
	steps := m.Steps
	if steps == nil {
		steps = []NodeMoveStep{}
	}
	v, err := json.Marshal(vols)
	if err != nil {
		return "", "", fmt.Errorf("store: marshal node move volumes: %w", err)
	}
	s, err := json.Marshal(steps)
	if err != nil {
		return "", "", fmt.Errorf("store: marshal node move steps: %w", err)
	}
	return string(v), string(s), nil
}

// NodeDrain is one persisted drain of a node.
type NodeDrain struct {
	ID           string
	NodeID       string
	TargetNodeID string
	Status       string
	Error        string
	Blocked      []NodeDrainBlocked
	StartedAt    string
	UpdatedAt    string
	FinishedAt   string
}

// NodeDrainBlocked is a resource a drain could not move, with the reason.
type NodeDrainBlocked struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

const nodeDrainColumns = `SELECT id, node_id, target_node_id, status, error, blocked, started_at, updated_at, finished_at FROM node_drains`

// CreateNodeDrain inserts a running drain, failing with ErrNodeMoveInProgress
// when the node is already draining.
func (db *DB) CreateNodeDrain(ctx context.Context, d NodeDrain) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_drains (id, node_id, target_node_id, status, error, blocked, started_at, updated_at, finished_at)
		VALUES (?, ?, ?, 'running', '', '[]', ?, ?, '')
	`, d.ID, d.NodeID, d.TargetNodeID, d.StartedAt, d.StartedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNodeMoveInProgress
		}
		return fmt.Errorf("store: create node drain %q: %w", d.ID, err)
	}
	return nil
}

// SaveNodeDrain persists a drain's mutable fields.
func (db *DB) SaveNodeDrain(ctx context.Context, d NodeDrain) error {
	blocked := d.Blocked
	if blocked == nil {
		blocked = []NodeDrainBlocked{}
	}
	b, err := json.Marshal(blocked)
	if err != nil {
		return fmt.Errorf("store: marshal drain blocked: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE node_drains SET status = ?, error = ?, blocked = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
		d.Status, d.Error, string(b), d.UpdatedAt, d.FinishedAt, d.ID)
	if err != nil {
		return fmt.Errorf("store: save node drain %q: %w", d.ID, err)
	}
	return requireOneRow(res, ErrNodeMoveNotFound, "save node drain "+d.ID)
}

// GetNodeDrain returns one drain by ID.
func (db *DB) GetNodeDrain(ctx context.Context, id string) (NodeDrain, error) {
	d, err := scanNodeDrain(db.QueryRowContext(ctx, nodeDrainColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeDrain{}, ErrNodeMoveNotFound
	}
	if err != nil {
		return NodeDrain{}, fmt.Errorf("store: get node drain %q: %w", id, err)
	}
	return d, nil
}

// ListNodeDrains returns drains newest first, for one node or (nodeID "")
// only the running ones across every node.
func (db *DB) ListNodeDrains(ctx context.Context, nodeID string, limit int) ([]NodeDrain, error) {
	q := nodeDrainColumns + ` WHERE status = 'running'`
	args := []any{}
	if nodeID != "" {
		q = nodeDrainColumns + ` WHERE node_id = ?`
		args = append(args, nodeID)
	}
	q += ` ORDER BY started_at DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list node drains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeDrain
	for rows.Next() {
		d, err := scanNodeDrain(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node drain: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanNodeDrain(scan func(dest ...any) error) (NodeDrain, error) {
	var d NodeDrain
	var blocked string
	if err := scan(&d.ID, &d.NodeID, &d.TargetNodeID, &d.Status, &d.Error, &blocked, &d.StartedAt, &d.UpdatedAt, &d.FinishedAt); err != nil {
		return NodeDrain{}, err
	}
	if err := json.Unmarshal([]byte(blocked), &d.Blocked); err != nil {
		return NodeDrain{}, fmt.Errorf("parse blocked: %w", err)
	}
	return d, nil
}
