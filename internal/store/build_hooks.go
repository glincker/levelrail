package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrBuildHookNotFound is returned when no build hook matches the id.
var ErrBuildHookNotFound = errors.New("store: build hook not found")

// ErrBuildHookExists is returned when an app already has a hook with that name.
var ErrBuildHookExists = errors.New("store: build hook name already used for this app")

// BuildHook is one named incoming trigger URL for an app (migrations/2570_build_hooks.sql).
type BuildHook struct {
	ID              string
	ServiceName     string
	Name            string
	Branch          string
	TokenHash       string
	CreatedAt       time.Time
	LastTriggeredAt *time.Time
	LastResult      string
	TriggerCount    int
}

const buildHookColumns = `id, service_name, name, branch, token_hash, created_at, last_triggered_at, last_result, trigger_count`

type buildHookScanner interface{ Scan(dest ...any) error }

func scanBuildHook(s buildHookScanner) (BuildHook, error) {
	var h BuildHook
	var created string
	var triggered sql.NullString
	if err := s.Scan(&h.ID, &h.ServiceName, &h.Name, &h.Branch, &h.TokenHash, &created, &triggered, &h.LastResult, &h.TriggerCount); err != nil {
		return BuildHook{}, err
	}
	var err error
	if h.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return BuildHook{}, fmt.Errorf("parse created_at: %w", err)
	}
	if h.LastTriggeredAt, err = parseTimePtr(triggered); err != nil {
		return BuildHook{}, err
	}
	return h, nil
}

// CreateBuildHook inserts h, rejecting a duplicate name for the same app.
func (db *DB) CreateBuildHook(ctx context.Context, h BuildHook) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO build_hooks (id, service_name, name, branch, token_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)
	`, h.ID, h.ServiceName, h.Name, h.Branch, h.TokenHash, h.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrBuildHookExists
		}
		return fmt.Errorf("store: create build hook for %q: %w", h.ServiceName, err)
	}
	return nil
}

// ListBuildHooks returns serviceName's hooks, oldest first.
func (db *DB) ListBuildHooks(ctx context.Context, serviceName string) ([]BuildHook, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+buildHookColumns+` FROM build_hooks WHERE service_name = ? ORDER BY created_at, id`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list build hooks for %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()
	var out []BuildHook
	for rows.Next() {
		h, err := scanBuildHook(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan build hook: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list build hooks for %q: %w", serviceName, err)
	}
	return out, nil
}

// GetBuildHook returns the hook with id, or ErrBuildHookNotFound.
func (db *DB) GetBuildHook(ctx context.Context, id string) (BuildHook, error) {
	h, err := scanBuildHook(db.QueryRowContext(ctx, `SELECT `+buildHookColumns+` FROM build_hooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return BuildHook{}, ErrBuildHookNotFound
	}
	if err != nil {
		return BuildHook{}, fmt.Errorf("store: get build hook %q: %w", id, err)
	}
	return h, nil
}

// SetBuildHookTokenHash replaces a hook's secret, invalidating the old URL.
func (db *DB) SetBuildHookTokenHash(ctx context.Context, serviceName, id, hash string) error {
	res, err := db.ExecContext(ctx, `UPDATE build_hooks SET token_hash = ? WHERE id = ? AND service_name = ?`, hash, id, serviceName)
	return buildHookAffected(res, err, "rotate", id)
}

// DeleteBuildHook revokes a hook.
func (db *DB) DeleteBuildHook(ctx context.Context, serviceName, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM build_hooks WHERE id = ? AND service_name = ?`, id, serviceName)
	return buildHookAffected(res, err, "delete", id)
}

// RecordBuildHookTrigger stamps the latest trigger time and outcome.
func (db *DB) RecordBuildHookTrigger(ctx context.Context, id, result string, at time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE build_hooks SET last_triggered_at = ?, last_result = ?, trigger_count = trigger_count + 1 WHERE id = ?
	`, at.UTC().Format(time.RFC3339), result, id)
	if err != nil {
		return fmt.Errorf("store: record build hook trigger %q: %w", id, err)
	}
	return nil
}

// RecordBuildHookResult stores the final outcome of the latest trigger.
func (db *DB) RecordBuildHookResult(ctx context.Context, id, result string) error {
	if _, err := db.ExecContext(ctx, `UPDATE build_hooks SET last_result = ? WHERE id = ?`, result, id); err != nil {
		return fmt.Errorf("store: record build hook result %q: %w", id, err)
	}
	return nil
}

func buildHookAffected(res sql.Result, err error, op, id string) error {
	if err != nil {
		return fmt.Errorf("store: %s build hook %q: %w", op, id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBuildHookNotFound
	}
	return nil
}
