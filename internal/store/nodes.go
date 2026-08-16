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
	NodeStatusPending NodeStatus = "pending"
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	// NodeStatusCordoned is unused as of TASKS.md 3.7: cordon's real
	// backing state is the Schedulable field / schedulable column
	// (migration 0010), a separate axis from Status, exactly matching
	// this constant's own original doc comment promise that a cordoned
	// node "can still be online." Left defined rather than removed,
	// since nothing before 3.7 ever set it and removing an exported
	// constant is a needless breaking change for zero benefit.
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

	// Schedulable is cordon's backing state (TASKS.md 3.7, migration
	// 0013): false means "unschedulable for new placements, but not
	// evacuated," an operator-initiated state independent of Status.
	// New nodes are always schedulable (SaveNode never trusts this
	// field, matching SaveDesiredService's own "not every field of the
	// input struct is honored, some have a dedicated mutation method"
	// convention); the only way to change it is SetNodeSchedulable.
	Schedulable bool

	// AcceptsAppWorkloads and AcceptsBuildWorkloads are TASKS.md 3.5's
	// node capability flags (migrations/0010_node_workloads.sql): which
	// kinds of work this node is willing to run, independent of each
	// other. A node can be either, both, or neither. See the migration's
	// own doc comment for the default values new and pre-existing nodes
	// get.
	AcceptsAppWorkloads   bool
	AcceptsBuildWorkloads bool

	// MeshPublicKey and MeshAddress are TASKS.md 3.4's WireGuard mesh
	// state (migrations/0014_node_mesh.sql). Both empty means this node
	// has not joined the mesh yet, which is a normal transient state for
	// a node that enrolled but has not brought a device up.
	//
	// Deliberately plain strings rather than internal/network's Key and
	// netip.Addr: this package stores rows, and having it import
	// internal/network to parse and validate them would put the mesh's
	// own rules (a key is 32 bytes, an address is inside the mesh CIDR)
	// in two places. internal/network.ValidateInventory is the one place
	// those rules live; the reconciler that reads these converts and
	// validates on the way in.
	MeshPublicKey string
	MeshAddress   string

	CreatedAt time.Time
	UpdatedAt time.Time
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
// re-checking" shape store.CreateUser's own doc comment already
// establishes for a different unique-constraint conflict.
func (db *DB) SaveNode(ctx context.Context, n Node) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (id, name, address, status, cert_fingerprint, joined_at, last_seen_at, schedulable, accepts_app_workloads, accepts_build_workloads, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
	`,
		n.ID, n.Name, n.Address, string(n.Status), n.CertFingerprint,
		formatTimePtr(n.JoinedAt), formatTimePtr(n.LastSeenAt),
		boolToInt(n.AcceptsAppWorkloads), boolToInt(n.AcceptsBuildWorkloads),
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
// Still no placement guard at this layer: internal/api's
// handleDeleteNode (TASKS.md 3.7) is what refuses to call this at all
// while ListDesiredServicesByNode/ListDesiredDatabasesByNode report any
// placements remaining, the same "guard at the API boundary, keep the
// store primitive unconditional" shape this package already uses
// elsewhere (e.g. SaveDesiredService's node_id exception is enforced by
// which method gets called, not by a check inside one shared method).
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

// UpdateNodeWorkloads sets a node's workload capability flags
// (TASKS.md 3.5). Returns ErrNodeNotFound if no such node exists, the
// same "distinguish real failure from a no-op" rigor UpdateNodeStatus
// already applies to its own conditional UPDATE.
func (db *DB) UpdateNodeWorkloads(ctx context.Context, id string, acceptsApp, acceptsBuild bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET accepts_app_workloads = ?, accepts_build_workloads = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, boolToInt(acceptsApp), boolToInt(acceptsBuild), id)
	if err != nil {
		return fmt.Errorf("store: update workloads for node %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update workloads for node %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// TouchNodeLastSeen updates last_seen_at to now, best-effort by design
// matching TouchAPITokenLastUsed: a heartbeat recording failure must
// never block whatever triggered it. Called once at session start
// (Server.Session) and, per TASKS.md 3.7, repeatedly on a fixed
// interval for as long as that session's stream stays open
// (Server.heartbeatLoop): a single call at connect time can't
// distinguish "still connected" from "connected an hour ago, then the
// process hung," which is exactly the case internal/reconcile/nodehealth
// needs to detect.
func (db *DB) TouchNodeLastSeen(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE nodes SET last_seen_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("store: touch node %q last_seen_at: %w", id, err)
	}
	return nil
}

// SetNodeSchedulable cordons (schedulable=false) or uncordons
// (schedulable=true) a node (TASKS.md 3.7). Returns ErrNodeNotFound if
// no such node exists, the same "distinguish real failure from a no-op"
// rigor UpdateNodeStatus applies to its own conditional UPDATE. Does not
// touch Status and does not evacuate anything already running there:
// cordon on its own only affects new placements (internal/api's
// handleSetAppNode and handleDrainNode both check Schedulable before
// accepting a target node), see this package's own Node.Schedulable doc
// comment for why that's a separate column from Status in the first
// place.
func (db *DB) SetNodeSchedulable(ctx context.Context, id string, schedulable bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET schedulable = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
	`, boolToInt(schedulable), id)
	if err != nil {
		return fmt.Errorf("store: set schedulable for node %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set schedulable for node %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// UpdateNodeMesh records a node's WireGuard identity and assigned mesh
// address (TASKS.md 3.4, migrations/0014_node_mesh.sql).
//
// Its own narrow updater rather than a field on SaveNode, matching
// UpdateNodeStatus and TouchNodeLastSeen: SaveNode is insert-only
// because a node's identity is fixed at enrollment, and mesh state
// changes on an entirely different schedule (every time the mesh
// coordinator learns something new about a node) than enrollment does.
//
// Returns ErrNodeNotFound if no such node exists, so a coordinator
// distributing to a node that was deleted mid-pass finds out rather than
// silently writing nothing.
func (db *DB) UpdateNodeMesh(ctx context.Context, id, publicKey, address string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes
		SET mesh_public_key = ?, mesh_address = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, publicKey, address, id)
	if err != nil {
		return fmt.Errorf("store: update mesh state for node %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update mesh state for node %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// boolToInt and intToBool convert Go's bool to and from SQLite's
// INTEGER 0/1 convention (SQLite has no native boolean type), the first
// boolean-shaped columns this package has needed to store: nodes'
// schedulable (migrations/0013_node_health.sql) and
// accepts_app_workloads/accepts_build_workloads (migrations/
// 0010_node_workloads.sql).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

const nodeSelectColumns = `
	SELECT id, name, address, status, cert_fingerprint, joined_at, last_seen_at, schedulable, accepts_app_workloads, accepts_build_workloads, mesh_public_key, mesh_address, created_at, updated_at`

func scanNode(scan func(dest ...any) error) (*Node, error) {
	var (
		n                                          Node
		status                                     string
		joinedAt, lastSeenAt                       sql.NullString
		schedulable                                int
		acceptsAppWorkloads, acceptsBuildWorkloads int
		createdAt, updatedAt                       string
	)
	if err := scan(&n.ID, &n.Name, &n.Address, &status, &n.CertFingerprint,
		&joinedAt, &lastSeenAt, &schedulable,
		&acceptsAppWorkloads, &acceptsBuildWorkloads,
		&n.MeshPublicKey, &n.MeshAddress,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	n.Status = NodeStatus(status)
	n.Schedulable = schedulable != 0
	n.AcceptsAppWorkloads = intToBool(acceptsAppWorkloads)
	n.AcceptsBuildWorkloads = intToBool(acceptsBuildWorkloads)

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
