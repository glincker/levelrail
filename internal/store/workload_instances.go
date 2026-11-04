package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrWorkloadInstanceNotFound is returned when no instance row matches.
var ErrWorkloadInstanceNotFound = errors.New("store: workload instance not found")

// ErrWorkloadInstanceStale is returned when a move names a generation that is no longer current.
var ErrWorkloadInstanceStale = errors.New("store: workload instance generation is stale")

// WorkloadInstance is one instance of a per-node or ordered parent service.
type WorkloadInstance struct {
	Service      string
	InstanceKey  string
	ChildService string
	// Ordinal is -1 for per-node instances.
	Ordinal      int
	NodeID       string
	State        string
	Generation   int64
	FencedNodeID string
	FencedReason string
	FencedAt     time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var workloadInstanceStates = map[string]bool{
	WorkloadInstancePending:  true,
	WorkloadInstanceActive:   true,
	WorkloadInstanceUpdating: true,
	WorkloadInstanceFenced:   true,
	WorkloadInstanceRetired:  true,
}

const workloadInstanceColumns = `service, instance_key, child_service, ordinal, node_id, state, generation,
	fenced_node_id, fenced_reason, fenced_at, created_at, updated_at`

func validWorkloadInstanceState(state string) error {
	if !workloadInstanceStates[state] {
		return fmt.Errorf("store: unknown workload instance state %q", state)
	}
	return nil
}

func storeNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseStoreTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func scanWorkloadInstance(row interface{ Scan(...any) error }) (WorkloadInstance, error) {
	var wi WorkloadInstance
	var fencedAt, created, updated string
	err := row.Scan(&wi.Service, &wi.InstanceKey, &wi.ChildService, &wi.Ordinal, &wi.NodeID, &wi.State,
		&wi.Generation, &wi.FencedNodeID, &wi.FencedReason, &fencedAt, &created, &updated)
	if err != nil {
		return WorkloadInstance{}, err
	}
	wi.FencedAt, wi.CreatedAt, wi.UpdatedAt = parseStoreTime(fencedAt), parseStoreTime(created), parseStoreTime(updated)
	return wi, nil
}

// SetServiceSetMembership marks a service as a child of parent, or clears it with a zero value.
func (db *DB) SetServiceSetMembership(ctx context.Context, name string, m ServiceSetMembership) error {
	if m.ParentService == name && name != "" {
		return fmt.Errorf("store: service %q cannot be its own parent", name)
	}
	if (m.ParentService == "") != (m.InstanceKey == "") {
		return fmt.Errorf("store: parent service and instance key must be set together")
	}
	res, err := db.ExecContext(ctx, `UPDATE desired_services SET parent_service = ?, instance_key = ? WHERE name = ?`,
		m.ParentService, m.InstanceKey, name)
	if err != nil {
		return fmt.Errorf("store: set membership of %s: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// GetServiceSetMembership returns the parent link of a service; a zero value means an ordinary service.
func (db *DB) GetServiceSetMembership(ctx context.Context, name string) (ServiceSetMembership, error) {
	var m ServiceSetMembership
	err := db.QueryRowContext(ctx, `SELECT parent_service, instance_key FROM desired_services WHERE name = ?`, name).
		Scan(&m.ParentService, &m.InstanceKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceSetMembership{}, ErrServiceNotFound
	}
	if err != nil {
		return ServiceSetMembership{}, fmt.Errorf("store: get membership of %s: %w", name, err)
	}
	return m, nil
}

// ListChildServiceNames returns the names of the services owned by parent, sorted.
func (db *DB) ListChildServiceNames(ctx context.Context, parent string) ([]string, error) {
	if parent == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM desired_services WHERE parent_service = ? ORDER BY name`, parent)
	if err != nil {
		return nil, fmt.Errorf("store: list children of %s: %w", parent, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan child of %s: %w", parent, err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpsertWorkloadInstance creates an instance row or updates its child, ordinal, node and state.
// The generation and fencing fields are only changed by Move and Fence, so a repair pass cannot reset them.
func (db *DB) UpsertWorkloadInstance(ctx context.Context, wi WorkloadInstance) error {
	if wi.Service == "" || wi.InstanceKey == "" || wi.ChildService == "" {
		return fmt.Errorf("store: workload instance needs service, instance key and child service")
	}
	if wi.State == "" {
		wi.State = WorkloadInstancePending
	}
	if err := validWorkloadInstanceState(wi.State); err != nil {
		return err
	}
	now := storeNow()
	_, err := db.ExecContext(ctx, `INSERT INTO workload_instances
		(service, instance_key, child_service, ordinal, node_id, state, generation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT (service, instance_key) DO UPDATE SET
			child_service = excluded.child_service, ordinal = excluded.ordinal, node_id = excluded.node_id,
			state = excluded.state, updated_at = excluded.updated_at`,
		wi.Service, wi.InstanceKey, wi.ChildService, wi.Ordinal, wi.NodeID, wi.State, now, now)
	if err != nil {
		return fmt.Errorf("store: upsert workload instance %s/%s: %w", wi.Service, wi.InstanceKey, err)
	}
	return nil
}

// GetWorkloadInstance returns one instance of service.
func (db *DB) GetWorkloadInstance(ctx context.Context, service, key string) (WorkloadInstance, error) {
	wi, err := scanWorkloadInstance(db.QueryRowContext(ctx,
		`SELECT `+workloadInstanceColumns+` FROM workload_instances WHERE service = ? AND instance_key = ?`, service, key))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkloadInstance{}, ErrWorkloadInstanceNotFound
	}
	if err != nil {
		return WorkloadInstance{}, fmt.Errorf("store: get workload instance %s/%s: %w", service, key, err)
	}
	return wi, nil
}

// GetWorkloadInstanceByChild returns the instance a child service backs, for the generation check on node recovery.
func (db *DB) GetWorkloadInstanceByChild(ctx context.Context, child string) (WorkloadInstance, error) {
	wi, err := scanWorkloadInstance(db.QueryRowContext(ctx,
		`SELECT `+workloadInstanceColumns+` FROM workload_instances WHERE child_service = ?`, child))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkloadInstance{}, ErrWorkloadInstanceNotFound
	}
	if err != nil {
		return WorkloadInstance{}, fmt.Errorf("store: get workload instance for child %s: %w", child, err)
	}
	return wi, nil
}

// ListWorkloadInstances returns the instances of service, ordinals first in order, then per-node keys.
func (db *DB) ListWorkloadInstances(ctx context.Context, service string) ([]WorkloadInstance, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+workloadInstanceColumns+` FROM workload_instances
		WHERE service = ? ORDER BY ordinal < 0, ordinal, instance_key`, service)
	if err != nil {
		return nil, fmt.Errorf("store: list workload instances of %s: %w", service, err)
	}
	defer func() { _ = rows.Close() }()
	var out []WorkloadInstance
	for rows.Next() {
		wi, err := scanWorkloadInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan workload instance of %s: %w", service, err)
		}
		out = append(out, wi)
	}
	return out, rows.Err()
}

// SetWorkloadInstanceState changes only the state; use FenceWorkloadInstance to fence.
func (db *DB) SetWorkloadInstanceState(ctx context.Context, service, key, state string) error {
	if err := validWorkloadInstanceState(state); err != nil {
		return err
	}
	if state == WorkloadInstanceFenced {
		return fmt.Errorf("store: fence an instance with FenceWorkloadInstance so the reason is kept")
	}
	res, err := db.ExecContext(ctx, `UPDATE workload_instances SET state = ?, updated_at = ? WHERE service = ? AND instance_key = ?`,
		state, storeNow(), service, key)
	if err != nil {
		return fmt.Errorf("store: set state of workload instance %s/%s: %w", service, key, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkloadInstanceNotFound
	}
	return nil
}

// FenceWorkloadInstance marks the instance fenced on its current node with a reason, keeping the generation.
func (db *DB) FenceWorkloadInstance(ctx context.Context, service, key, reason string) error {
	now := storeNow()
	res, err := db.ExecContext(ctx, `UPDATE workload_instances SET state = ?, fenced_node_id = node_id,
		fenced_reason = ?, fenced_at = ?, updated_at = ? WHERE service = ? AND instance_key = ?`,
		WorkloadInstanceFenced, reason, now, now, service, key)
	if err != nil {
		return fmt.Errorf("store: fence workload instance %s/%s: %w", service, key, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkloadInstanceNotFound
	}
	return nil
}

// MoveWorkloadInstance moves an instance to nodeID and bumps its generation, only if the caller saw
// expectedGeneration. Two racing moves cannot both win, and the stale copy is detectable by generation.
func (db *DB) MoveWorkloadInstance(ctx context.Context, service, key, nodeID string, expectedGeneration int64) (int64, error) {
	var gen int64
	err := db.QueryRowContext(ctx, `UPDATE workload_instances SET node_id = ?, generation = generation + 1,
		state = ?, fenced_node_id = '', fenced_reason = '', fenced_at = '', updated_at = ?
		WHERE service = ? AND instance_key = ? AND generation = ? RETURNING generation`,
		nodeID, WorkloadInstancePending, storeNow(), service, key, expectedGeneration).Scan(&gen)
	if errors.Is(err, sql.ErrNoRows) {
		if _, gerr := db.GetWorkloadInstance(ctx, service, key); gerr != nil {
			return 0, gerr
		}
		return 0, ErrWorkloadInstanceStale
	}
	if err != nil {
		return 0, fmt.Errorf("store: move workload instance %s/%s: %w", service, key, err)
	}
	return gen, nil
}

// DeleteWorkloadInstance removes one instance row; deleting the parent service removes all of them.
func (db *DB) DeleteWorkloadInstance(ctx context.Context, service, key string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM workload_instances WHERE service = ? AND instance_key = ?`, service, key)
	if err != nil {
		return fmt.Errorf("store: delete workload instance %s/%s: %w", service, key, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkloadInstanceNotFound
	}
	return nil
}
