package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvConfigRevisionsMax caps the revisions kept per app; older ones are pruned.
const (
	EnvConfigRevisionsMax     = "APP_CONFIG_REVISIONS_MAX"
	DefaultConfigRevisionsMax = 100
)

// AppConfigRevision is one stored version of an app definition. Snapshot and
// Changes are JSON owned by internal/configrev; the store never reads inside.
type AppConfigRevision struct {
	App      string
	N        int
	At       time.Time
	Actor    string
	RevertOf int
	Snapshot string
	Changes  string
}

func configRevisionsMax() int {
	if n, err := strconv.Atoi(os.Getenv(EnvConfigRevisionsMax)); err == nil && n > 0 {
		return n
	}
	return DefaultConfigRevisionsMax
}

// AddAppConfigRevision appends the next revision for the app and prunes the
// oldest beyond the cap. It returns the new revision number.
func (db *DB) AddAppConfigRevision(ctx context.Context, rev AppConfigRevision) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: add config revision for %q: %w", rev.App, err)
	}
	defer func() { _ = tx.Rollback() }()
	var last sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(n) FROM app_config_revisions WHERE app_name = ?`, rev.App).Scan(&last); err != nil {
		return 0, fmt.Errorf("store: add config revision for %q: %w", rev.App, err)
	}
	n := int(last.Int64) + 1
	if rev.At.IsZero() {
		rev.At = time.Now()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_config_revisions (app_name, n, at, actor, revert_of, snapshot, changes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rev.App, n, rev.At.UTC().Format(time.RFC3339Nano), rev.Actor, rev.RevertOf, rev.Snapshot, rev.Changes); err != nil {
		return 0, fmt.Errorf("store: add config revision for %q: %w", rev.App, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_config_revisions WHERE app_name = ? AND n <= ?`, rev.App, n-configRevisionsMax()); err != nil {
		return 0, fmt.Errorf("store: prune config revisions for %q: %w", rev.App, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: add config revision for %q: %w", rev.App, err)
	}
	return n, nil
}

// ListAppConfigRevisions returns the app's revisions newest first.
func (db *DB) ListAppConfigRevisions(ctx context.Context, app string, limit int) ([]AppConfigRevision, error) {
	if limit <= 0 {
		limit = configRevisionsMax()
	}
	rows, err := db.reader().QueryContext(ctx, `
		SELECT app_name, n, at, actor, revert_of, snapshot, changes
		FROM app_config_revisions WHERE app_name = ? ORDER BY n DESC LIMIT ?
	`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list config revisions for %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppConfigRevision
	for rows.Next() {
		rev, err := scanConfigRevision(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

// GetAppConfigRevision returns revision n, or ok false when it is absent.
func (db *DB) GetAppConfigRevision(ctx context.Context, app string, n int) (AppConfigRevision, bool, error) {
	rev, err := scanConfigRevision(db.QueryRowContext(ctx, `
		SELECT app_name, n, at, actor, revert_of, snapshot, changes
		FROM app_config_revisions WHERE app_name = ? AND n = ?
	`, app, n).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return AppConfigRevision{}, false, nil
	}
	return rev, err == nil, err
}

func scanConfigRevision(scan func(...any) error) (AppConfigRevision, error) {
	var rev AppConfigRevision
	var at string
	if err := scan(&rev.App, &rev.N, &at, &rev.Actor, &rev.RevertOf, &rev.Snapshot, &rev.Changes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rev, err
		}
		return rev, fmt.Errorf("store: scan config revision: %w", err)
	}
	rev.At, _ = time.Parse(time.RFC3339Nano, at)
	return rev, nil
}
