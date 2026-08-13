package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NodeStatus is a node's current lifecycle state.
type NodeStatus string

// The four states a node can be in. Pending is the state a newly
// enrolled node starts in (join token exchanged, no heartbeat received
// yet, TASKS.md 3.2 territory); Online/Offline track heartbeat presence
// (TASKS.md 3.7); Cordoned means "unschedulable for new placements, but
// not evacuated" (also 3.7), a distinct axis from Online/Offline, not a
// replacement for it: a cordoned node can still be online.
const (
	NodeStatusPending  NodeStatus = "pending"
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusCordoned NodeStatus = "cordoned"
)

// Node is one managed machine in the fleet (TASKS.md Phase 3). A row is
// created by the join-token enrollment flow, not by this package's own
// CRUD surface: SaveNode exists as a real, directly testable primitive
// for that future enrollment code (TASKS.md 3.2) to call, but nothing in
// internal/api wires an operator-facing "create a node" HTTP route to it
// yet, deliberately, per this migration's own doc comment.
type Node struct {
	ID              string
	Name            string
	Address         string
	Status          NodeStatus
	CertFingerprint string
	JoinedAt        *time.Time
	LastSeenAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ErrNodeNotFound is returned by GetNode when no node has that ID.
var ErrNodeNotFound = errors.New("store: node not found")

// ErrNodeNameTaken is returned by SaveNode when name is already used by
// a different node: node names are operator-facing identifiers (shown
// in the join command, the node list), so collisions need to surface as
// a clear error, not a silently overwritten row.
var ErrNodeNameTaken = errors.New("store: node name already taken")

// SaveNode inserts a new node row. Unlike SaveDesiredService, this is
// insert-only: a node's identity (ID, name) is fixed at enrollment time,
// there is no "replace this node's full record" operation, matching how
// SaveAPIToken (0007) never updates a token in place either. Status
// transitions go through UpdateNodeStatus, heartbeats through
// TouchNodeLastSeen, both narrower and safer than a caller re-supplying
// a whole Node struct on every heartbeat.
//
// On an INSERT failure, this re-checks whether name is already taken
// (via nodeNameExists) to distinguish ErrNodeNameTaken from a genuine,
// unrelated database error, rather than parsing the driver's own error
// type: the same "let the constraint decide, then classify by
// re-checking" shape CreateAdminUser's own doc comment already
// establishes for a different unique-constraint conflict.
func (db *DB) SaveNode(ctx context.Context, n Node) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (id, name, address, status, cert_fingerprint, joined_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		n.ID, n.Name, n.Address, string(n.Status), n.CertFingerprint,
		formatTimePtr(n.JoinedAt), formatTimePtr(n.LastSeenAt),
		formatTime(n.CreatedAt), formatTime(n.UpdatedAt),
	)
	if err == nil {
		return nil
	}

	exists, existsErr := db.nodeNameExists(ctx, n.Name)
	if existsErr == nil && exists {
		return ErrNodeNameTaken
	}
	return fmt.Errorf("store: save node %q: %w", n.ID, err)
}

func (db *DB) nodeNameExists(ctx context.Context, name string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM nodes WHERE name = ? LIMIT 1`, name).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check node name %q exists: %w", name, err)
	}
	return true, nil
}

// GetNode returns the node with this ID, or ErrNodeNotFound.
func (db *DB) GetNode(ctx context.Context, id string) (*Node, error) {
	row := db.QueryRowContext(ctx, nodeSelectColumns+` FROM nodes WHERE id = ?`, id)
	n, err := scanNode(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get node %q: %w", id, err)
	}
	return n, nil
}

// ListNodes returns every node, ordered by name.
func (db *DB) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := db.QueryContext(ctx, nodeSelectColumns+` FROM nodes ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node row: %w", err)
		}
		out = append(out, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate node rows: %w", err)
	}
	return out, nil
}

// DeleteNode removes a node by ID. Idempotent, matching
// DeleteDesiredService's convention: deleting an already-gone node is
// not an error.
//
// No guard here against a node that still has services placed on it:
// TASKS.md 3.3 (placement) hasn't landed yet, so no service can be
// assigned to a node at all as of this pass, making that guard
// currently vacuous rather than simply unwritten. TASKS.md 3.7's own
// scope note already calls out closing this once drain exists; tracked
// there, not silently forgotten here.
func (db *DB) DeleteNode(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete node %q: %w", id, err)
	}
	return nil
}

// UpdateNodeStatus sets a node's status. Returns ErrNodeNotFound if no
// such node exists, the same "distinguish real failure from a no-op"
// rigor RevokeAPIToken applies to its own conditional UPDATE.
func (db *DB) UpdateNodeStatus(ctx context.Context, id string, status NodeStatus) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, string(status), id)
	if err != nil {
		return fmt.Errorf("store: update status for node %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update status for node %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// TouchNodeLastSeen updates last_seen_at to now, best-effort by design
// matching TouchAPITokenLastUsed: a heartbeat recording failure must
// never block whatever triggered it.
func (db *DB) TouchNodeLastSeen(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE nodes SET last_seen_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("store: touch node %q last_seen_at: %w", id, err)
	}
	return nil
}

const nodeSelectColumns = `
	SELECT id, name, address, status, cert_fingerprint, joined_at, last_seen_at, created_at, updated_at`

func scanNode(scan func(dest ...any) error) (*Node, error) {
	var (
		n                    Node
		status               string
		joinedAt, lastSeenAt sql.NullString
		createdAt, updatedAt string
	)
	if err := scan(&n.ID, &n.Name, &n.Address, &status, &n.CertFingerprint,
		&joinedAt, &lastSeenAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	n.Status = NodeStatus(status)

	var err error
	n.JoinedAt, err = parseTimePtr(joinedAt)
	if err != nil {
		return nil, fmt.Errorf("parse joined_at: %w", err)
	}
	n.LastSeenAt, err = parseTimePtr(lastSeenAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_seen_at: %w", err)
	}
	n.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	n.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &n, nil
}
