package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTagNotFound is returned by GetTag, DeleteTag, AttachAppTag, and
// DetachAppTag when id doesn't match any row.
var ErrTagNotFound = errors.New("store: tag not found")

// ErrTagNameTaken is SaveTag's failure mode when name is already used by
// a different tag (tags.name's own UNIQUE constraint).
var ErrTagNameTaken = errors.New("store: tag name already exists")

// Tag is an arbitrary, operator-defined label an app can be attached to
// for organization and filtering, independent of the project/
// environment hierarchy (migrations/0106_tags.sql).
type Tag struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// SaveTag inserts a new tag row. ID is minted by the caller
// (internal/api), the same "generate before the INSERT" pattern
// SaveFeatureFlag's own doc comment establishes. On a name conflict it
// re-checks by name to classify the failure, the same pattern
// CreateUser's own doc comment establishes for a different unique
// constraint.
func (db *DB) SaveTag(ctx context.Context, t Tag) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO tags (id, name, created_at) VALUES (?, ?, ?)
	`, t.ID, t.Name, formatTime(t.CreatedAt))
	if err == nil {
		return nil
	}
	if _, getErr := db.GetTagByName(ctx, t.Name); getErr == nil {
		return ErrTagNameTaken
	}
	return fmt.Errorf("store: save tag %q: %w", t.ID, err)
}

// GetTag returns the tag with this ID, or ErrTagNotFound.
func (db *DB) GetTag(ctx context.Context, id string) (Tag, error) {
	row := db.QueryRowContext(ctx, `SELECT id, name, created_at FROM tags WHERE id = ?`, id)
	t, err := scanTag(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrTagNotFound
	}
	if err != nil {
		return Tag{}, fmt.Errorf("store: get tag %q: %w", id, err)
	}
	return *t, nil
}

// GetTagByName returns the tag with this name, or ErrTagNotFound.
func (db *DB) GetTagByName(ctx context.Context, name string) (Tag, error) {
	row := db.QueryRowContext(ctx, `SELECT id, name, created_at FROM tags WHERE name = ?`, name)
	t, err := scanTag(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrTagNotFound
	}
	if err != nil {
		return Tag{}, fmt.Errorf("store: get tag by name %q: %w", name, err)
	}
	return *t, nil
}

// ListTags returns every tag, alphabetical by name so a filter control's
// own option list doesn't need to sort it again client-side.
func (db *DB) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	return scanTags(rows)
}

// DeleteTag removes a tag row; app_tags rows referencing it cascade-delete
// (migrations/0106_tags.sql's own ON DELETE CASCADE), so an app that was
// tagged with it just loses that one attachment, nothing else. Returns
// ErrTagNotFound if id doesn't exist.
func (db *DB) DeleteTag(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete tag %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrTagNotFound, "delete tag %q", id)
}

// AttachAppTag attaches tagID to appName, idempotently: attaching an
// already-attached tag is a no-op, not a conflict, the same
// "declarative, not incremental" convention SetAppVaultEnv's own doc
// comment establishes for a different attach-style write.
func (db *DB) AttachAppTag(ctx context.Context, tagID, appName string) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO app_tags (tag_id, app_name, created_at) VALUES (?, ?, ?)
	`, tagID, appName, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: attach tag %q to app %q: %w", tagID, appName, err)
	}
	return nil
}

// DetachAppTag removes tagID from appName. Unlike AttachAppTag this is
// not silently idempotent: detaching a tag that was never attached
// returns ErrTagNotFound, so a caller's DELETE gets a real 404 instead
// of a false-positive success, the same distinction ClearAppVaultEnv's
// own doc comment draws for its own detach endpoint.
func (db *DB) DetachAppTag(ctx context.Context, tagID, appName string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM app_tags WHERE tag_id = ? AND app_name = ?`, tagID, appName)
	if err != nil {
		return fmt.Errorf("store: detach tag %q from app %q: %w", tagID, appName, err)
	}
	return rowsAffectedOrNotFound(res, ErrTagNotFound, "detach tag from app %q", tagID+"/"+appName)
}

// ListTagsForApp returns every tag attached to appName, alphabetical by
// name, the same ordering ListTags uses.
func (db *DB) ListTagsForApp(ctx context.Context, appName string) ([]Tag, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id, t.name, t.created_at
		FROM tags t
		JOIN app_tags at ON at.tag_id = t.id
		WHERE at.app_name = ?
		ORDER BY t.name
	`, appName)
	if err != nil {
		return nil, fmt.Errorf("store: list tags for app %q: %w", appName, err)
	}
	return scanTags(rows)
}

// ListTagsForApps batch-loads every tag attached to any app in
// appNames, keyed by app name, so GET /api/v1/apps can render tag chips
// on every row from one query instead of one ListTagsForApp call per
// app (the same N+1 GET .../deploys gap appListResource's own doc
// comment describes for status, avoided here the same way). An app with
// no tags simply has no key in the returned map.
func (db *DB) ListTagsForApps(ctx context.Context, appNames []string) (map[string][]Tag, error) {
	out := make(map[string][]Tag, len(appNames))
	if len(appNames) == 0 {
		return out, nil
	}

	// strings.Builder rather than fmt.Sprintf, the same reasoning
	// GetConditionsForControllers' own doc comment gives: only the
	// number of "?" placeholders varies with input, but gosec's G201
	// pattern-matches on fmt.Sprintf feeding a query string regardless.
	var query strings.Builder
	query.WriteString(`
		SELECT at.app_name, t.id, t.name, t.created_at
		FROM app_tags at
		JOIN tags t ON t.id = at.tag_id
		WHERE at.app_name IN (`)
	args := make([]any, len(appNames))
	for i, name := range appNames {
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString("?")
		args[i] = name
	}
	query.WriteString(`)
		ORDER BY at.app_name, t.name
	`)
	rows, err := db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("store: batch list tags for apps: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var appName string
		t, err := scanTag(func(dest ...any) error {
			return rows.Scan(&appName, dest[0], dest[1], dest[2])
		})
		if err != nil {
			return nil, fmt.Errorf("store: scan batch tag row: %w", err)
		}
		out[appName] = append(out[appName], *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate batch tag rows: %w", err)
	}
	return out, nil
}

// ListAppNamesByTag returns the name of every app tagID is attached to,
// alphabetical, the primitive GET /api/v1/tags/{id}/apps filters by.
func (db *DB) ListAppNamesByTag(ctx context.Context, tagID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT app_name FROM app_tags WHERE tag_id = ? ORDER BY app_name
	`, tagID)
	if err != nil {
		return nil, fmt.Errorf("store: list apps for tag %q: %w", tagID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan app name for tag %q: %w", tagID, err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app names for tag %q: %w", tagID, err)
	}
	return out, nil
}

func scanTags(rows *sql.Rows) ([]Tag, error) {
	defer func() {
		_ = rows.Close()
	}()

	var out []Tag
	for rows.Next() {
		t, err := scanTag(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan tag row: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tag rows: %w", err)
	}
	return out, nil
}

func scanTag(scan func(dest ...any) error) (*Tag, error) {
	var (
		t         Tag
		createdAt string
	)
	if err := scan(&t.ID, &t.Name, &createdAt); err != nil {
		return nil, err
	}
	var err error
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &t, nil
}
