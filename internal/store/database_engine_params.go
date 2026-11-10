package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DatabaseEngineParams is the applied, staged and previous tuning sets of
// one managed database. Pending is nil when nothing is staged.
type DatabaseEngineParams struct {
	Applied  map[string]string
	Pending  map[string]string
	Previous map[string]string
	StagedBy string
	StagedAt string
	// AppliedAt is when the running set was last promoted, empty before any apply.
	AppliedAt string
	// RolledBackAt and RolledBackReason record the last automatic rollback;
	// they are cleared by the next apply.
	RolledBackAt     string
	RolledBackReason string
	// PendingRaw is the stored pending JSON, nil when NULL, for compare and set writes.
	PendingRaw *string
}

// ErrEngineParamsConflict reports that the staged set changed since it was read.
var ErrEngineParamsConflict = errors.New("store: engine params changed concurrently")

func pendingArg(raw *string) any {
	if raw == nil {
		return nil
	}
	return *raw
}

// EngineParamSet is a parameter map kept as canonical JSON so DesiredDatabase
// stays comparable; the zero value means no parameters.
type EngineParamSet string

func newEngineParamSet(m map[string]string) EngineParamSet {
	if len(m) == 0 {
		return ""
	}
	raw, err := encodeEngineParams(m)
	if err != nil {
		return ""
	}
	return EngineParamSet(raw)
}

// Map decodes the set; it is never nil.
func (s EngineParamSet) Map() map[string]string { return decodeEngineParams(string(s)) }

func decodeEngineParams(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return out
}

func encodeEngineParams(m map[string]string) (string, error) {
	if m == nil {
		m = map[string]string{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encode engine params: %w", err)
	}
	return string(b), nil
}

// GetDatabaseEngineParams loads the parameter sets of name.
func (db *DB) GetDatabaseEngineParams(ctx context.Context, name string) (DatabaseEngineParams, error) {
	var applied, previous string
	var pending sql.NullString
	var p DatabaseEngineParams
	err := db.QueryRowContext(ctx, `SELECT engine_params, engine_params_pending, engine_params_previous, engine_params_staged_by, engine_params_staged_at, engine_params_applied_at, engine_params_rolled_back_at, engine_params_rolled_back_reason FROM desired_databases WHERE name = ?`, name).
		Scan(&applied, &pending, &previous, &p.StagedBy, &p.StagedAt, &p.AppliedAt, &p.RolledBackAt, &p.RolledBackReason)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrDatabaseNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: get engine params for database %q: %w", name, err)
	}
	p.Applied = decodeEngineParams(applied)
	p.Previous = decodeEngineParams(previous)
	if pending.Valid {
		p.Pending = decodeEngineParams(pending.String)
		raw := pending.String
		p.PendingRaw = &raw
	}
	return p, nil
}

// StageDatabaseEngineParams records the full next set without touching the
// running one, only if the staged set is still expected (nil means none).
func (db *DB) StageDatabaseEngineParams(ctx context.Context, name string, params map[string]string, by string, expected *string) error {
	raw, err := encodeEngineParams(params)
	if err != nil {
		return err
	}
	return db.execDatabaseRow(ctx, name, "stage engine params",
		`UPDATE desired_databases SET engine_params_pending = ?, engine_params_staged_by = ?, engine_params_staged_at = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params_pending IS ?`,
		raw, by, time.Now().UTC().Format(time.RFC3339), name, pendingArg(expected))
}

// DiscardDatabaseEngineParams drops the staged set if it is still expected.
func (db *DB) DiscardDatabaseEngineParams(ctx context.Context, name string, expected *string) error {
	return db.execDatabaseRow(ctx, name, "discard engine params",
		`UPDATE desired_databases SET engine_params_pending = NULL, engine_params_staged_by = '', engine_params_staged_at = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params_pending IS ?`, name, pendingArg(expected))
}

// ApplyDatabaseEngineParams promotes exactly the staged set the caller read
// and keeps the old one for rollback. applied is false when it changed or is gone.
func (db *DB) ApplyDatabaseEngineParams(ctx context.Context, name, expected string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE desired_databases SET engine_params_previous = engine_params, engine_params = engine_params_pending, engine_params_pending = NULL, engine_params_staged_by = '', engine_params_staged_at = '', engine_params_applied_at = ?, engine_params_rolled_back_at = '', engine_params_rolled_back_reason = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params_pending = ?`, time.Now().UTC().Format(time.RFC3339Nano), name, expected)
	if err != nil {
		return false, fmt.Errorf("store: apply engine params for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: apply engine params for database %q: rows affected: %w", name, err)
	}
	return n > 0, nil
}

func (db *DB) execDatabaseRow(ctx context.Context, name, what, query string, args ...any) error {
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: %s for database %q: %w", what, name, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: %s for database %q: rows affected: %w", what, name, err)
	} else if n == 0 {
		return ErrEngineParamsConflict
	}
	return nil
}

// AuditActionDatabaseParamsAutoRollback is the system audit action of an
// automatic rollback; the reconciler writes it with the rollback itself.
const AuditActionDatabaseParamsAutoRollback = "database.params.auto_rollback"

// RollbackDatabaseEngineParams swaps the running and previous sets when the
// running set and its apply time are still expected and no rollback happened
// since; the failed set stays in previous for re-staging. Audit is in the same transaction.
func (db *DB) RollbackDatabaseEngineParams(ctx context.Context, name string, expected map[string]string, expectedAppliedAt, reason string, at time.Time, audit AuditEntry) (rolled bool, err error) {
	raw, err := encodeEngineParams(expected)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: begin engine params rollback: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE desired_databases SET engine_params = engine_params_previous, engine_params_previous = engine_params, engine_params_rolled_back_at = ?, engine_params_rolled_back_reason = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params = ? AND engine_params_applied_at = ? AND engine_params_rolled_back_at = '' AND engine_params_previous <> engine_params`,
		at.UTC().Format(time.RFC3339), reason, name, raw, expectedAppliedAt)
	if err != nil {
		return false, fmt.Errorf("store: roll back engine params for database %q: %w", name, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if err := insertAuditEntry(ctx, tx, audit); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: commit engine params rollback: %w", err)
	}
	return true, nil
}

// DropDatabaseEngineParams replaces the running set with keep when it is
// still expected, with its audit row in the same transaction. previous
// becomes keep too, so no later rollback restores a refused parameter.
func (db *DB) DropDatabaseEngineParams(ctx context.Context, name string, expected, keep map[string]string, audit AuditEntry) error {
	return db.dropEngineParams(ctx, name, "", "", expected, keep, audit)
}

// SetDatabaseVersionDroppingEngineParams changes the version from fromVersion
// to toVersion and drops refused parameters in one transaction, so neither
// happens without the other.
func (db *DB) SetDatabaseVersionDroppingEngineParams(ctx context.Context, name, fromVersion, toVersion string, expected, keep map[string]string, audit AuditEntry) error {
	return db.dropEngineParams(ctx, name, fromVersion, toVersion, expected, keep, audit)
}

// SetDatabaseVersionIfEngineParams changes the version only while the running
// set is still expected, so a concurrent apply cannot slip past re-validation.
func (db *DB) SetDatabaseVersionIfEngineParams(ctx context.Context, name, fromVersion, toVersion string, expected map[string]string) error {
	raw, err := encodeEngineParams(expected)
	if err != nil {
		return err
	}
	return db.execDatabaseRow(ctx, name, "set version",
		`UPDATE desired_databases SET version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND version = ? AND engine_params = ?`,
		toVersion, name, fromVersion, raw)
}

func (db *DB) dropEngineParams(ctx context.Context, name, fromVersion, toVersion string, expected, keep map[string]string, audit AuditEntry) error {
	oldRaw, err := encodeEngineParams(expected)
	if err != nil {
		return err
	}
	newRaw, err := encodeEngineParams(keep)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin engine params drop: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := `UPDATE desired_databases SET engine_params = ?, engine_params_previous = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params = ?`
	args := []any{newRaw, newRaw, name, oldRaw}
	if toVersion != "" {
		query = `UPDATE desired_databases SET engine_params = ?, engine_params_previous = ?, version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params = ? AND version = ?`
		args = []any{newRaw, newRaw, toVersion, name, oldRaw, fromVersion}
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: drop engine params for database %q: %w", name, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: drop engine params for database %q: rows affected: %w", name, err)
	} else if n == 0 {
		return ErrEngineParamsConflict
	}
	if err := insertAuditEntry(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit engine params drop: %w", err)
	}
	return nil
}
