package store

import (
	"context"
	"fmt"
)

// SetLBUpstreamAdminState stores the admin state for one replica. The state
// "active" removes the row; the service must exist.
func (db *DB) SetLBUpstreamAdminState(ctx context.Context, service string, replica int, state string) error {
	if state == "active" {
		if _, err := db.ExecContext(ctx, `DELETE FROM lb_upstream_admin_state WHERE service_name = ? AND replica = ?`, service, replica); err != nil {
			return fmt.Errorf("store: clear lb upstream admin state: %w", err)
		}
		return nil
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO lb_upstream_admin_state (service_name, replica, admin_state)
		SELECT name, ?, ? FROM desired_services WHERE name = ?
		ON CONFLICT (service_name, replica) DO UPDATE SET
			admin_state = excluded.admin_state,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, replica, state, service)
	if err != nil {
		return fmt.Errorf("store: set lb upstream admin state: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// ListLBUpstreamAdminStates returns every non-active state as
// service -> replica -> state.
func (db *DB) ListLBUpstreamAdminStates(ctx context.Context) (map[string]map[int]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT service_name, replica, admin_state FROM lb_upstream_admin_state`)
	if err != nil {
		return nil, fmt.Errorf("store: list lb upstream admin states: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]map[int]string)
	for rows.Next() {
		var service, state string
		var replica int
		if err := rows.Scan(&service, &replica, &state); err != nil {
			return nil, fmt.Errorf("store: scan lb upstream admin state: %w", err)
		}
		if out[service] == nil {
			out[service] = map[int]string{}
		}
		out[service][replica] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate lb upstream admin states: %w", err)
	}
	return out, nil
}
