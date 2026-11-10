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
	err := db.QueryRowContext(ctx, `SELECT engine_params, engine_params_pending, engine_params_previous, engine_params_staged_by, engine_params_staged_at FROM desired_databases WHERE name = ?`, name).
		Scan(&applied, &pending, &previous, &p.StagedBy, &p.StagedAt)
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
	res, err := db.ExecContext(ctx, `UPDATE desired_databases SET engine_params_previous = engine_params, engine_params = engine_params_pending, engine_params_pending = NULL, engine_params_staged_by = '', engine_params_staged_at = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND engine_params_pending = ?`, name, expected)
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
