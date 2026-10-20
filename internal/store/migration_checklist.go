package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrMigrationChecklistNotFound is returned for an app with no checklist.
var ErrMigrationChecklistNotFound = errors.New("store: migration checklist not found")

// MigrationChecklist is one app's stored cutover checklist. StepsJSON is
// owned by internal/migrationplan.
type MigrationChecklist struct {
	AppName   string
	Platform  string
	SourceID  string
	StepsJSON string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const checklistColumns = `app_name, platform, source_id, steps_json, created_at, updated_at`

func scanChecklist(sc interface{ Scan(...any) error }) (MigrationChecklist, error) {
	var c MigrationChecklist
	var created, updated string
	if err := sc.Scan(&c.AppName, &c.Platform, &c.SourceID, &c.StepsJSON, &created, &updated); err != nil {
		return MigrationChecklist{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, nil
}

// SaveMigrationChecklist inserts or replaces an app's checklist.
func (db *DB) SaveMigrationChecklist(ctx context.Context, c MigrationChecklist) error {
	_, err := db.ExecContext(ctx, `INSERT INTO migration_checklists (`+checklistColumns+`) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET platform = excluded.platform, source_id = excluded.source_id,
		steps_json = excluded.steps_json, updated_at = excluded.updated_at`,
		c.AppName, c.Platform, c.SourceID, c.StepsJSON, c.CreatedAt.UTC().Format(time.RFC3339Nano), c.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save migration checklist %q: %w", c.AppName, err)
	}
	return nil
}

// GetMigrationChecklist returns one app's checklist.
func (db *DB) GetMigrationChecklist(ctx context.Context, app string) (MigrationChecklist, error) {
	c, err := scanChecklist(db.QueryRowContext(ctx, `SELECT `+checklistColumns+` FROM migration_checklists WHERE app_name = ?`, app))
	if errors.Is(err, sql.ErrNoRows) {
		return MigrationChecklist{}, ErrMigrationChecklistNotFound
	}
	if err != nil {
		return MigrationChecklist{}, fmt.Errorf("store: get migration checklist %q: %w", app, err)
	}
	return c, nil
}

// ListMigrationChecklists returns every checklist by app name.
func (db *DB) ListMigrationChecklists(ctx context.Context) ([]MigrationChecklist, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+checklistColumns+` FROM migration_checklists ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list migration checklists: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MigrationChecklist
	for rows.Next() {
		c, err := scanChecklist(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan migration checklist: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
