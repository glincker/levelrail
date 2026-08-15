package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Supported managed database engines, matching internal/spec's list:
// Postgres and Redis shipped as first-class resources in the initial
// release, MySQL joined once internal/reconcile/database's controller
// grew a third engine case.
const (
	EnginePostgres = "postgres"
	EngineRedis    = "redis"
	EngineMySQL    = "mysql"
)

// DesiredDatabase is what a future database controller (TASKS.md 1.8)
// reconciles a managed database container against. No credentials field:
// see the comment in migrations/0003_desired_databases.sql for why.
type DesiredDatabase struct {
	Name    string
	Engine  string
	Version string
	// NodeID: see DesiredService.NodeID's own doc comment, identical
	// meaning and identical "SaveDesiredDatabase never writes it, only
	// UpdateDatabaseNode does" exception below.
	NodeID string
	// ProjectID: see DesiredService.ProjectID's own doc comment,
	// identical meaning and identical "SaveDesiredDatabase never writes
	// it, only UpdateDatabaseProject does" exception below.
	ProjectID string
	// BackupTargetID, BackupSchedule, BackupRetain: wave-2 roadmap item 6,
	// scheduled backups (migrations/0023_scheduled_backups.sql). Same
	// "SaveDesiredDatabase never writes it, only SetDatabaseBackupSchedule
	// does" exception NodeID/ProjectID already establish above, applied
	// to the three columns internal/backup.Scheduler reads every tick.
	// BackupTargetID is "" when no backup_targets row is configured
	// (SQL NULL, mirroring ProjectID's own empty-string-means-NULL
	// convention); BackupSchedule is "" when no cron schedule is set
	// (SQL '', mirroring NodeID's own empty-string-is-the-real-value
	// convention, see the migration's own doc comment for why the two
	// columns deliberately use different null-ness conventions);
	// BackupRetain is 0, meaning "keep every successful backup, prune
	// nothing."
	BackupTargetID string
	BackupSchedule string
	BackupRetain   int
}

// SaveDesiredDatabase creates or fully replaces the desired state for a
// named database, the same whole-record-replacement semantics as
// SaveDesiredService, including the identical NodeID exception
// SaveDesiredService's own doc comment explains.
func (db *DB) SaveDesiredDatabase(ctx context.Context, d DesiredDatabase) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO desired_databases (name, engine, version, node_id, project_id, updated_at)
		VALUES (?, ?, ?, '', NULL, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			engine = excluded.engine,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, d.Name, d.Engine, d.Version)
	if err != nil {
		return fmt.Errorf("store: save desired database %q: %w", d.Name, err)
	}
	return nil
}

// UpdateDatabaseNode is DesiredDatabase's counterpart to
// UpdateServiceNode.
func (db *DB) UpdateDatabaseNode(ctx context.Context, name, nodeID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_databases SET node_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nodeID, name)
	if err != nil {
		return fmt.Errorf("store: update node for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update node for database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// UpdateDatabaseProject is DesiredDatabase's counterpart to
// UpdateServiceProject: see that method's own doc comment for why an
// empty projectID is written as SQL NULL, not the empty string node_id
// uses.
func (db *DB) UpdateDatabaseProject(ctx context.Context, name, projectID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_databases SET project_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: projectID, Valid: projectID != ""}, name)
	if err != nil {
		return fmt.Errorf("store: update project for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update project for database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// ErrDatabaseNotFound is returned by GetDesiredDatabase when no database
// has that name.
var ErrDatabaseNotFound = errors.New("store: database not found")

// GetDesiredDatabase returns the desired state for name, or
// ErrDatabaseNotFound if no such database has been saved.
func (db *DB) GetDesiredDatabase(ctx context.Context, name string) (*DesiredDatabase, error) {
	row := db.QueryRowContext(ctx, `
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain
		FROM desired_databases
		WHERE name = ?
	`, name)

	d, err := scanDesiredDatabase(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDatabaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get desired database %q: %w", name, err)
	}
	return d, nil
}

// DeleteDesiredDatabase removes a database's desired state, the same
// "not found" sentinel and "does not stop the running container itself"
// gap DeleteDesiredService's own doc comment documents, applied to a
// database instead of a service.
func (db *DB) DeleteDesiredDatabase(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM desired_databases WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete desired database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete desired database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// ListDesiredDatabases returns every saved database, ordered by name.
func (db *DB) ListDesiredDatabases(ctx context.Context) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain
		FROM desired_databases
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list desired databases: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredDatabase
	for rows.Next() {
		d, err := scanDesiredDatabase(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired database row: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired database rows: %w", err)
	}
	return out, nil
}

// ListDesiredDatabasesByNode returns every saved database currently
// placed on nodeID, ordered by name. The database-kind counterpart to
// ListDesiredServicesByNode, same TASKS.md 3.7 drain/delete-guard
// callers.
func (db *DB) ListDesiredDatabasesByNode(ctx context.Context, nodeID string) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain
		FROM desired_databases
		WHERE node_id = ?
		ORDER BY name
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired databases for node %q: %w", nodeID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredDatabase
	for rows.Next() {
		d, err := scanDesiredDatabase(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired database row: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired database rows: %w", err)
	}
	return out, nil
}

// SetDatabaseBackupSchedule is DesiredDatabase's counterpart to
// UpdateDatabaseNode/UpdateDatabaseProject: a scheduled backup's config
// is set through its own endpoint and its own method, the same
// separation-from-ordinary-desired-state-saves those two already
// establish, rather than folding it into SaveDesiredDatabase's own
// whole-record replace. Passing targetID="" and schedule="" (the
// DELETE /api/v1/databases/{name}/backup-schedule path, retain also
// forced to 0 by the caller in that case) clears scheduled backups for
// name entirely: internal/backup.Scheduler's own ListScheduledDatabases
// query treats an empty schedule exactly like it was never configured,
// so this is a real "unset," not a sentinel every future caller has to
// remember to special-case.
func (db *DB) SetDatabaseBackupSchedule(ctx context.Context, name, targetID, schedule string, retain int) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_databases
		SET backup_target_id = ?, backup_schedule = ?, backup_retain = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE name = ?
	`, sql.NullString{String: targetID, Valid: targetID != ""}, schedule, retain, name)
	if err != nil {
		return fmt.Errorf("store: set backup schedule for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set backup schedule for database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// ListScheduledDatabases returns every database with both a non-empty
// backup_schedule and a resolved backup_target_id, ordered by name. The
// two are deliberately required together, not backup_schedule alone: a
// schedule whose backup_targets row was since deleted
// (migrations/0023's own ON DELETE SET NULL) can never actually run, and
// requiring both here means internal/backup.Scheduler.Tick never has to
// special-case that half-configured state itself, on every tick, for the
// lifetime of the process. Re-derived fresh on every call, the same
// "never cache, re-derive every pass" principle dynamicSource
// (cmd/levelrail/main.go) already applies to which databases exist at
// all.
func (db *DB) ListScheduledDatabases(ctx context.Context) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain
		FROM desired_databases
		WHERE backup_schedule != '' AND backup_target_id IS NOT NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled databases: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredDatabase
	for rows.Next() {
		d, err := scanDesiredDatabase(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan scheduled database row: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled database rows: %w", err)
	}
	return out, nil
}

// scanDesiredDatabase reads the column shape every desired_databases
// read method queries (GetDesiredDatabase, ListDesiredDatabases,
// ListDesiredDatabasesByNode, ListScheduledDatabases), via either
// row.Scan or rows.Scan (same signature), so the nullable-column
// handling exists exactly once. Mirrors scanDesiredService's shape in
// service.go, this package's own precedent for the identical problem.
func scanDesiredDatabase(scan func(dest ...any) error) (*DesiredDatabase, error) {
	var (
		d                         DesiredDatabase
		projectID, backupTargetID sql.NullString
	)
	if err := scan(&d.Name, &d.Engine, &d.Version, &d.NodeID, &projectID, &backupTargetID, &d.BackupSchedule, &d.BackupRetain); err != nil {
		return nil, err
	}
	d.ProjectID = projectID.String
	d.BackupTargetID = backupTargetID.String
	return &d, nil
}
