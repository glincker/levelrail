package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrComposeRevisionNotFound is returned when an app has no matching revision.
var ErrComposeRevisionNotFound = errors.New("store: compose revision not found")

// Compose revision sources.
const (
	ComposeRevisionSourceAPI      = "api"
	ComposeRevisionSourceTemplate = "template"
	ComposeRevisionSourceRollback = "rollback"
)

// ComposeRevision is one compose file a stack was deployed from.
type ComposeRevision struct {
	AppID      string
	Revision   int
	Compose    string
	Profiles   []string
	Source     string
	TemplateID string
	CreatedBy  string
	CreatedAt  time.Time
}

// AddComposeRevision stores rev as appID's next revision number and
// returns that number. Revision and CreatedAt on rev are ignored.
func (db *DB) AddComposeRevision(ctx context.Context, rev ComposeRevision) (int, error) {
	profiles, err := json.Marshal(nonNilSlice(rev.Profiles))
	if err != nil {
		return 0, fmt.Errorf("store: marshal compose profiles for %q: %w", rev.AppID, err)
	}
	source := rev.Source
	if source == "" {
		source = ComposeRevisionSourceAPI
	}
	var next int
	err = db.QueryRowContext(ctx, `
		INSERT INTO compose_revisions (app_id, revision, compose, profiles, source, template_id, created_by, created_at)
		VALUES (?, (SELECT COALESCE(MAX(revision), 0) + 1 FROM compose_revisions WHERE app_id = ?), ?, ?, ?, ?, ?, ?)
		RETURNING revision
	`, rev.AppID, rev.AppID, rev.Compose, string(profiles), source, rev.TemplateID, rev.CreatedBy, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("store: add compose revision for %q: %w", rev.AppID, err)
	}
	return next, nil
}

// ListComposeRevisions returns appID's revisions, newest first, at most limit (0 means all).
func (db *DB) ListComposeRevisions(ctx context.Context, appID string, limit int) ([]ComposeRevision, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := db.QueryContext(ctx, `
		SELECT app_id, revision, compose, profiles, source, template_id, created_by, created_at
		FROM compose_revisions WHERE app_id = ? ORDER BY revision DESC LIMIT ?
	`, appID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list compose revisions for %q: %w", appID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []ComposeRevision
	for rows.Next() {
		rev, err := scanComposeRevision(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan compose revision for %q: %w", appID, err)
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list compose revisions for %q: %w", appID, err)
	}
	return out, nil
}

// GetComposeRevision returns one revision; revision 0 means the latest.
func (db *DB) GetComposeRevision(ctx context.Context, appID string, revision int) (ComposeRevision, error) {
	query := `SELECT app_id, revision, compose, profiles, source, template_id, created_by, created_at
		FROM compose_revisions WHERE app_id = ? AND revision = ?`
	args := []any{appID, revision}
	if revision == 0 {
		query = `SELECT app_id, revision, compose, profiles, source, template_id, created_by, created_at
		FROM compose_revisions WHERE app_id = ? ORDER BY revision DESC LIMIT 1`
		args = []any{appID}
	}
	rev, err := scanComposeRevision(db.QueryRowContext(ctx, query, args...).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ComposeRevision{}, ErrComposeRevisionNotFound
	}
	if err != nil {
		return ComposeRevision{}, fmt.Errorf("store: get compose revision %d for %q: %w", revision, appID, err)
	}
	return rev, nil
}

// ListComposeApps returns every app ID with at least one compose revision, sorted.
func (db *DB) ListComposeApps(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT app_id FROM compose_revisions ORDER BY app_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list compose apps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan compose app: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list compose apps: %w", err)
	}
	return out, nil
}

// DeleteComposeRevisions removes every revision of appID.
func (db *DB) DeleteComposeRevisions(ctx context.Context, appID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM compose_revisions WHERE app_id = ?`, appID); err != nil {
		return fmt.Errorf("store: delete compose revisions for %q: %w", appID, err)
	}
	return nil
}

func scanComposeRevision(scan func(dest ...any) error) (ComposeRevision, error) {
	var (
		rev       ComposeRevision
		profiles  string
		createdAt string
	)
	if err := scan(&rev.AppID, &rev.Revision, &rev.Compose, &profiles, &rev.Source, &rev.TemplateID, &rev.CreatedBy, &createdAt); err != nil {
		return ComposeRevision{}, err
	}
	if err := json.Unmarshal([]byte(profiles), &rev.Profiles); err != nil {
		return ComposeRevision{}, fmt.Errorf("unmarshal profiles: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ComposeRevision{}, fmt.Errorf("parse created_at: %w", err)
	}
	rev.CreatedAt = t
	return rev, nil
}
