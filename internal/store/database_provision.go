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

// pendingDatabaseNamesSQL selects databases whose newest provisioning attempt
// has not reached ready. Databases with no attempt (created before
// provisioning steps existed) are not pending.
const pendingDatabaseNamesSQL = `
	SELECT e.database_name FROM database_provision_events e
	WHERE e.id = (SELECT MAX(x.id) FROM database_provision_events x WHERE x.database_name = e.database_name)
	AND NOT EXISTS (SELECT 1 FROM database_provision_events r WHERE r.attempt_id = e.attempt_id AND r.step = 'ready' AND r.status = 'done')`

// PendingDatabaseNames returns the databases that have no running container
// yet because creation is still in progress or failed.
func (db *DB) PendingDatabaseNames(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, pendingDatabaseNamesSQL)
	if err != nil {
		return nil, fmt.Errorf("store: list pending databases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan pending database: %w", err)
		}
		out[n] = true
	}
	return out, rows.Err()
}

// SetDatabaseExistingVolume stores the owner's choice about a leftover volume.
func (db *DB) SetDatabaseExistingVolume(ctx context.Context, name, choice string) error {
	res, err := db.ExecContext(ctx, `UPDATE desired_databases SET existing_volume = ?, volume_resolved = 0 WHERE name = ?`, choice, name)
	if err != nil {
		return fmt.Errorf("store: set existing volume choice for database %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// SetDatabaseVolumeResolved latches that the leftover-volume question is settled.
func (db *DB) SetDatabaseVolumeResolved(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET volume_resolved = 1 WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: mark volume resolved for database %q: %w", name, err)
	}
	return nil
}
