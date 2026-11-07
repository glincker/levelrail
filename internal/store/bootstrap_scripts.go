package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BootstrapScript is a saved shell snippet appended to the first-boot script of a new cloud node.
// The body is opaque text: the control plane stores it and never runs it.
type BootstrapScript struct {
	ID        string
	Name      string
	Body      string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	// ErrBootstrapScriptNotFound is returned when an id matches no script.
	ErrBootstrapScriptNotFound = errors.New("store: bootstrap script not found")
	// ErrBootstrapScriptNameTaken is returned when a name is already used by another script.
	ErrBootstrapScriptNameTaken = errors.New("store: bootstrap script name already exists")
)

const bootstrapScriptCols = `id, name, body, created_by, created_at, updated_at`

// CreateBootstrapScript inserts a script; the caller mints the id.
func (db *DB) CreateBootstrapScript(ctx context.Context, s BootstrapScript) error {
	_, err := db.ExecContext(ctx, `INSERT INTO bootstrap_scripts (`+bootstrapScriptCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Body, s.CreatedBy, formatTime(s.CreatedAt), formatTime(s.UpdatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: bootstrap_scripts.name") {
			return ErrBootstrapScriptNameTaken
		}
		return fmt.Errorf("store: create bootstrap script %q: %w", s.ID, err)
	}
	return nil
}

// GetBootstrapScript returns one script or ErrBootstrapScriptNotFound.
func (db *DB) GetBootstrapScript(ctx context.Context, id string) (BootstrapScript, error) {
	s, err := scanBootstrapScript(db.QueryRowContext(ctx, `SELECT `+bootstrapScriptCols+` FROM bootstrap_scripts WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return BootstrapScript{}, ErrBootstrapScriptNotFound
	}
	if err != nil {
		return BootstrapScript{}, fmt.Errorf("store: get bootstrap script %q: %w", id, err)
	}
	return s, nil
}

// ListBootstrapScripts returns every script ordered by name.
func (db *DB) ListBootstrapScripts(ctx context.Context) ([]BootstrapScript, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+bootstrapScriptCols+` FROM bootstrap_scripts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list bootstrap scripts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BootstrapScript
	for rows.Next() {
		s, err := scanBootstrapScript(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan bootstrap script: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate bootstrap scripts: %w", err)
	}
	return out, nil
}

// CountBootstrapScripts returns how many scripts are saved.
func (db *DB) CountBootstrapScripts(ctx context.Context) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bootstrap_scripts`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count bootstrap scripts: %w", err)
	}
	return n, nil
}

// UpdateBootstrapScript replaces name and body.
func (db *DB) UpdateBootstrapScript(ctx context.Context, id, name, body string, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE bootstrap_scripts SET name = ?, body = ?, updated_at = ? WHERE id = ?`, name, body, formatTime(now), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: bootstrap_scripts.name") {
			return ErrBootstrapScriptNameTaken
		}
		return fmt.Errorf("store: update bootstrap script %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrBootstrapScriptNotFound, "update bootstrap script %q", id)
}

// DeleteBootstrapScript removes a script.
func (db *DB) DeleteBootstrapScript(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM bootstrap_scripts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete bootstrap script %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrBootstrapScriptNotFound, "delete bootstrap script %q", id)
}

func scanBootstrapScript(scan func(dest ...any) error) (BootstrapScript, error) {
	var s BootstrapScript
	var created, updated string
	if err := scan(&s.ID, &s.Name, &s.Body, &s.CreatedBy, &created, &updated); err != nil {
		return BootstrapScript{}, err
	}
	var err error
	if s.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return BootstrapScript{}, fmt.Errorf("parse created_at: %w", err)
	}
	if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return BootstrapScript{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return s, nil
}
