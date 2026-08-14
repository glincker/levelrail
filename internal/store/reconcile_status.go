package store

import (
	"context"
	"fmt"
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
