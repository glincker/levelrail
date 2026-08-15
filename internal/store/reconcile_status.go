package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// UpsertConditions persists every condition from a controller's most
// recent Reconcile result. Every reconcile must emit a status condition
// that gets stored and shown in the UI; this is that storage.
// reconcile.Engine itself only ever held results in memory, lost on
// restart.
func (db *DB) UpsertConditions(ctx context.Context, controllerName string, conditions []reconcile.Condition) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: upsert conditions: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	for _, c := range conditions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reconcile_status (controller_name, condition_type, status, reason, message, updated_at)
			VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (controller_name, condition_type) DO UPDATE SET
				status = excluded.status,
				reason = excluded.reason,
				message = excluded.message,
				updated_at = excluded.updated_at
		`, controllerName, c.Type, string(c.Status), c.Reason, c.Message); err != nil {
			return fmt.Errorf("store: upsert condition %s/%s: %w", controllerName, c.Type, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: upsert conditions: commit: %w", err)
	}
	return nil
}

// GetConditions returns every stored condition for a controller, ordered
// by condition type, or nil if none have been recorded yet.
func (db *DB) GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT condition_type, status, reason, message, updated_at
		FROM reconcile_status
		WHERE controller_name = ?
		ORDER BY condition_type
	`, controllerName)
	if err != nil {
		return nil, fmt.Errorf("store: get conditions for %s: %w", controllerName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var conditions []reconcile.Condition
	for rows.Next() {
		var c reconcile.Condition
		var status, updatedAt string
		if err := rows.Scan(&c.Type, &status, &c.Reason, &c.Message, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan condition row: %w", err)
		}
		c.Status = reconcile.ConditionStatus(status)
		t, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse updated_at %q: %w", updatedAt, err)
		}
		c.LastTransitionTime = t
		conditions = append(conditions, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate condition rows: %w", err)
	}

	return conditions, nil
}

// GetConditionsForControllers is GetConditions' batched counterpart: one
// query for every controller in controllerNames, keyed by controller
// name in the returned map, instead of a GetConditions call per
// controller. Built for GET /api/v1/apps (internal/api/apps.go's
// handleListApps), which needs a status summary per app without
// reintroducing the N+1 AppRow.tsx's own doc comment warns against. A
// controller with no stored conditions is simply absent from the map,
// the same "nil means none recorded yet" convention GetConditions
// already establishes for a single controller.
func (db *DB) GetConditionsForControllers(ctx context.Context, controllerNames []string) (map[string][]reconcile.Condition, error) {
	result := make(map[string][]reconcile.Condition, len(controllerNames))
	if len(controllerNames) == 0 {
		return result, nil
	}

	// Built with strings.Builder rather than fmt.Sprintf: only the
	// number of "?" placeholders varies with input, never a value, but
	// gosec's G201 pattern-matches on fmt.Sprintf feeding a query string
	// regardless of what's actually being interpolated.
	var query strings.Builder
	query.WriteString(`
		SELECT controller_name, condition_type, status, reason, message, updated_at
		FROM reconcile_status
		WHERE controller_name IN (`)
	args := make([]any, len(controllerNames))
	for i, name := range controllerNames {
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString("?")
		args[i] = name
	}
	query.WriteString(`)
		ORDER BY controller_name, condition_type
	`)

	rows, err := db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("store: get conditions for %d controllers: %w", len(controllerNames), err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var controllerName string
		var c reconcile.Condition
		var status, updatedAt string
		if err := rows.Scan(&controllerName, &c.Type, &status, &c.Reason, &c.Message, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan batched condition row: %w", err)
		}
		c.Status = reconcile.ConditionStatus(status)
		t, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse updated_at %q: %w", updatedAt, err)
		}
		c.LastTransitionTime = t
		result[controllerName] = append(result[controllerName], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate batched condition rows: %w", err)
	}

	return result, nil
}
