package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrSavedQueryNotFound is returned when a saved query does not exist for the caller.
var ErrSavedQueryNotFound = errors.New("store: saved query not found")

// ErrSavedQueryExists is returned when the caller already has a saved query with that name.
var ErrSavedQueryExists = errors.New("store: saved query name already exists")

// DatabaseQueryHistory is one console execution (migrations/0389).
type DatabaseQueryHistory struct {
	ID            string
	DatabaseName  string
	PrincipalType string
	PrincipalID   string
	Mode          string
	SQL           string
	Fingerprint   string
	OK            bool
	Error         string
	DurationMs    int64
	RowCount      int
	CreatedAt     time.Time
}

// DatabaseSavedQuery is a named statement owned by one principal (migrations/0390).
type DatabaseSavedQuery struct {
	ID            string
	DatabaseName  string
	PrincipalType string
	PrincipalID   string
	Name          string
	SQL           string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func newQueryID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate %s id: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// AddDatabaseQueryHistory records an execution and trims the caller's
// history for that database to keep rows.
func (db *DB) AddDatabaseQueryHistory(ctx context.Context, h DatabaseQueryHistory, keep int) (DatabaseQueryHistory, error) {
	id, err := newQueryID("qh_")
	if err != nil {
		return DatabaseQueryHistory{}, err
	}
	h.ID = id
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now()
	}
	ok := 0
	if h.OK {
		ok = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO database_query_history (id, database_name, principal_type, principal_id, mode, sql_text, fingerprint, ok, error, duration_ms, row_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, h.ID, h.DatabaseName, h.PrincipalType, h.PrincipalID, h.Mode, h.SQL, h.Fingerprint, ok, h.Error, h.DurationMs, h.RowCount, h.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return DatabaseQueryHistory{}, fmt.Errorf("store: add query history for database %q: %w", h.DatabaseName, err)
	}
	if keep > 0 {
		_, err = db.ExecContext(ctx, `
			DELETE FROM database_query_history
			WHERE database_name = ? AND principal_type = ? AND principal_id = ? AND id NOT IN (
				SELECT id FROM database_query_history
				WHERE database_name = ? AND principal_type = ? AND principal_id = ?
				ORDER BY created_at DESC, id DESC LIMIT ?)
		`, h.DatabaseName, h.PrincipalType, h.PrincipalID, h.DatabaseName, h.PrincipalType, h.PrincipalID, keep)
		if err != nil {
			return h, fmt.Errorf("store: trim query history for database %q: %w", h.DatabaseName, err)
		}
	}
	return h, nil
}

// ListDatabaseQueryHistory returns the caller's history for a database, newest first.
func (db *DB) ListDatabaseQueryHistory(ctx context.Context, database, principalType, principalID string, limit int) ([]DatabaseQueryHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, principal_type, principal_id, mode, sql_text, fingerprint, ok, error, duration_ms, row_count, created_at
		FROM database_query_history
		WHERE database_name = ? AND principal_type = ? AND principal_id = ?
		ORDER BY created_at DESC, id DESC LIMIT ?
	`, database, principalType, principalID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list query history for database %q: %w", database, err)
	}
	defer func() { _ = rows.Close() }()

	out := []DatabaseQueryHistory{}
	for rows.Next() {
		var h DatabaseQueryHistory
		var ok int
		var created string
		if err := rows.Scan(&h.ID, &h.DatabaseName, &h.PrincipalType, &h.PrincipalID, &h.Mode, &h.SQL, &h.Fingerprint, &ok, &h.Error, &h.DurationMs, &h.RowCount, &created); err != nil {
			return nil, fmt.Errorf("store: scan query history: %w", err)
		}
		h.OK = ok == 1
		h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list query history for database %q: %w", database, err)
	}
	return out, nil
}

// ClearDatabaseQueryHistory deletes the caller's history for a database.
func (db *DB) ClearDatabaseQueryHistory(ctx context.Context, database, principalType, principalID string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM database_query_history WHERE database_name = ? AND principal_type = ? AND principal_id = ?
	`, database, principalType, principalID); err != nil {
		return fmt.Errorf("store: clear query history for database %q: %w", database, err)
	}
	return nil
}

// SaveDatabaseQuery creates a saved query, or ErrSavedQueryExists when the name is taken.
func (db *DB) SaveDatabaseQuery(ctx context.Context, q DatabaseSavedQuery) (DatabaseSavedQuery, error) {
	id, err := newQueryID("sq_")
	if err != nil {
		return DatabaseSavedQuery{}, err
	}
	q.ID = id
	now := time.Now()
	q.CreatedAt, q.UpdatedAt = now, now
	var exists int
	err = db.QueryRowContext(ctx, `
		SELECT 1 FROM database_saved_queries WHERE database_name = ? AND principal_type = ? AND principal_id = ? AND name = ?
	`, q.DatabaseName, q.PrincipalType, q.PrincipalID, q.Name).Scan(&exists)
	if err == nil {
		return DatabaseSavedQuery{}, ErrSavedQueryExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return DatabaseSavedQuery{}, fmt.Errorf("store: check saved query %q: %w", q.Name, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO database_saved_queries (id, database_name, principal_type, principal_id, name, sql_text, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, q.ID, q.DatabaseName, q.PrincipalType, q.PrincipalID, q.Name, q.SQL, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return DatabaseSavedQuery{}, fmt.Errorf("store: save query %q for database %q: %w", q.Name, q.DatabaseName, err)
	}
	return q, nil
}

// ListDatabaseSavedQueries returns the caller's saved queries for a database, by name.
func (db *DB) ListDatabaseSavedQueries(ctx context.Context, database, principalType, principalID string) ([]DatabaseSavedQuery, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, principal_type, principal_id, name, sql_text, created_at, updated_at
		FROM database_saved_queries
		WHERE database_name = ? AND principal_type = ? AND principal_id = ? ORDER BY name
	`, database, principalType, principalID)
	if err != nil {
		return nil, fmt.Errorf("store: list saved queries for database %q: %w", database, err)
	}
	defer func() { _ = rows.Close() }()

	out := []DatabaseSavedQuery{}
	for rows.Next() {
		var q DatabaseSavedQuery
		var created, updated string
		if err := rows.Scan(&q.ID, &q.DatabaseName, &q.PrincipalType, &q.PrincipalID, &q.Name, &q.SQL, &created, &updated); err != nil {
			return nil, fmt.Errorf("store: scan saved query: %w", err)
		}
		q.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		q.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list saved queries for database %q: %w", database, err)
	}
	return out, nil
}

// DeleteDatabaseSavedQuery removes one of the caller's saved queries.
func (db *DB) DeleteDatabaseSavedQuery(ctx context.Context, database, principalType, principalID, id string) error {
	res, err := db.ExecContext(ctx, `
		DELETE FROM database_saved_queries WHERE id = ? AND database_name = ? AND principal_type = ? AND principal_id = ?
	`, id, database, principalType, principalID)
	if err != nil {
		return fmt.Errorf("store: delete saved query %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedQueryNotFound
	}
	return nil
}
