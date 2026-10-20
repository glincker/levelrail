package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Visibility values of a saved log query.
const (
	SavedLogQueryPrivate  = "private"
	SavedLogQueryInstance = "instance"
)

// ErrSavedLogQueryNotFound is returned when no saved log query has the id.
var ErrSavedLogQueryNotFound = errors.New("store: saved log query not found")

// ErrSavedLogQueryExists is returned when the owner already has a query with that name for the app.
var ErrSavedLogQueryExists = errors.New("store: saved log query name already used")

// SavedLogQuery is a named log search. SpecJSON is opaque to the store.
type SavedLogQuery struct {
	ID         string
	Name       string
	App        string
	SpecJSON   string
	OwnerType  string
	OwnerID    string
	OwnerName  string
	Visibility string
	Pinned     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const savedLogQueryColumns = `id, name, app, spec_json, owner_type, owner_id, owner_name, visibility, pinned, created_at, updated_at`

// NewSavedLogQueryID returns a fresh saved log query id.
func NewSavedLogQueryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate saved log query id: %w", err)
	}
	return "slq_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateSavedLogQuery inserts q.
func (db *DB) CreateSavedLogQuery(ctx context.Context, q SavedLogQuery) error {
	now := time.Now().UTC()
	q.CreatedAt, q.UpdatedAt = now, now
	_, err := db.ExecContext(ctx, `INSERT INTO saved_log_queries (`+savedLogQueryColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.ID, q.Name, q.App, q.SpecJSON, q.OwnerType, q.OwnerID, q.OwnerName, q.Visibility, q.Pinned,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrSavedLogQueryExists
		}
		return fmt.Errorf("store: create saved log query %q: %w", q.ID, err)
	}
	return nil
}

// UpdateSavedLogQuery replaces name, app, spec, visibility and pinned.
func (db *DB) UpdateSavedLogQuery(ctx context.Context, q SavedLogQuery) error {
	res, err := db.ExecContext(ctx, `UPDATE saved_log_queries SET name = ?, app = ?, spec_json = ?,
		visibility = ?, pinned = ?, updated_at = ? WHERE id = ?`,
		q.Name, q.App, q.SpecJSON, q.Visibility, q.Pinned, time.Now().UTC().Format(time.RFC3339Nano), q.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrSavedLogQueryExists
		}
		return fmt.Errorf("store: update saved log query %q: %w", q.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedLogQueryNotFound
	}
	return nil
}

// GetSavedLogQuery returns one saved log query or ErrSavedLogQueryNotFound.
func (db *DB) GetSavedLogQuery(ctx context.Context, id string) (SavedLogQuery, error) {
	q, err := scanSavedLogQuery(db.QueryRowContext(ctx, `SELECT `+savedLogQueryColumns+` FROM saved_log_queries WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedLogQuery{}, ErrSavedLogQueryNotFound
	}
	if err != nil {
		return SavedLogQuery{}, fmt.Errorf("store: get saved log query %q: %w", id, err)
	}
	return q, nil
}

// ListSavedLogQueries returns queries owned by ownerID or shared instance
// wide, newest first. app narrows to one app's queries plus the unscoped ones.
func (db *DB) ListSavedLogQueries(ctx context.Context, ownerID, app string) ([]SavedLogQuery, error) {
	query := `SELECT ` + savedLogQueryColumns + ` FROM saved_log_queries WHERE (owner_id = ? OR visibility = ?)`
	args := []any{ownerID, SavedLogQueryInstance}
	if app != "" {
		query += ` AND (app = ? OR app = '')`
		args = append(args, app)
	}
	query += ` ORDER BY pinned DESC, updated_at DESC LIMIT 500`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list saved log queries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SavedLogQuery
	for rows.Next() {
		q, err := scanSavedLogQuery(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan saved log query: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate saved log queries: %w", err)
	}
	return out, nil
}

// DeleteSavedLogQuery removes one saved log query.
func (db *DB) DeleteSavedLogQuery(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM saved_log_queries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete saved log query %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedLogQueryNotFound
	}
	return nil
}

func scanSavedLogQuery(r rowScanner) (SavedLogQuery, error) {
	var q SavedLogQuery
	var created, updated string
	if err := r.Scan(&q.ID, &q.Name, &q.App, &q.SpecJSON, &q.OwnerType, &q.OwnerID, &q.OwnerName, &q.Visibility, &q.Pinned, &created, &updated); err != nil {
		return q, err
	}
	q.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	q.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return q, nil
}
