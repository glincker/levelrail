package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// DatabaseProvisionEvent is one step transition while a database is created.
type DatabaseProvisionEvent struct {
	ID        int64
	Database  string
	AttemptID string
	Step      string
	Status    string
	Detail    string
	Current   int64
	Total     int64
	At        time.Time
}

// AppendDatabaseProvisionEvent records e; a zero At means now.
func (db *DB) AppendDatabaseProvisionEvent(ctx context.Context, e DatabaseProvisionEvent) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_provision_events (database_name, attempt_id, step, status, detail, current, total, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, e.Database, e.AttemptID, e.Step, e.Status, e.Detail, e.Current, e.Total, e.At.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: append provision event %q for database %q: %w", e.Step, e.Database, err)
	}
	return nil
}

// BeginDatabaseProvisionAttempt starts a new attempt for name and keeps only
// the newest keep attempts.
func (db *DB) BeginDatabaseProvisionAttempt(ctx context.Context, name string, keep int) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: provision attempt id: %w", err)
	}
	id := "prov_" + hex.EncodeToString(buf)
	if keep < 1 {
		keep = 1
	}
	_, err := db.ExecContext(ctx, `
		DELETE FROM database_provision_events
		WHERE database_name = ? AND attempt_id NOT IN (
			SELECT attempt_id FROM database_provision_events WHERE database_name = ?
			GROUP BY attempt_id ORDER BY MAX(id) DESC LIMIT ?)
	`, name, name, keep-1)
	if err != nil {
		return "", fmt.Errorf("store: prune provision attempts for %q: %w", name, err)
	}
	return id, nil
}

// LatestDatabaseProvisionAttempt returns the newest attempt's id and events
// in order, or an empty id when the database never had one.
func (db *DB) LatestDatabaseProvisionAttempt(ctx context.Context, name string) (string, []DatabaseProvisionEvent, error) {
	var id string
	err := db.QueryRowContext(ctx, `
		SELECT attempt_id FROM database_provision_events WHERE database_name = ? ORDER BY id DESC LIMIT 1
	`, name).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("store: latest provision attempt for %q: %w", name, err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, attempt_id, step, status, detail, current, total, at
		FROM database_provision_events WHERE attempt_id = ? ORDER BY id ASC
	`, id)
	if err != nil {
		return "", nil, fmt.Errorf("store: list provision events for %q: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	var out []DatabaseProvisionEvent
	for rows.Next() {
		var (
			e  DatabaseProvisionEvent
			at string
		)
		if err := rows.Scan(&e.ID, &e.Database, &e.AttemptID, &e.Step, &e.Status, &e.Detail, &e.Current, &e.Total, &at); err != nil {
			return "", nil, fmt.Errorf("store: scan provision event: %w", err)
		}
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return "", nil, fmt.Errorf("store: iterate provision events: %w", err)
	}
	return id, out, nil
}
