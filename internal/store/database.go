package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Supported managed database engines. internal/spec's own engine list
// (app.yaml declarative support) is tracked separately and not
// guaranteed to match this one: Dragonfly and ClickHouse are available
// here but not yet in internal/spec.
const (
	EnginePostgres   = "postgres"
	EngineRedis      = "redis"
	EngineMySQL      = "mysql"
	EngineMongoDB    = "mongodb"
	EngineMariaDB    = "mariadb"
	EngineKeyDB      = "keydb"
	EngineDragonfly  = "dragonfly"
	EngineClickHouse = "clickhouse"
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
	// PubliclyAccessible, PublicPort: whether this database's container
	// port is bound to a host port so an operator's own database GUI
	// tool can connect to it directly, not just from inside the Docker
	// network (migrations/0026_database_public_access.sql). Same
	// "SaveDesiredDatabase never writes it, only
	// SetDatabasePublicAccess does" exception NodeID/ProjectID/
	// BackupSchedule already establish. PublicPort is 0 when
	// PubliclyAccessible is false (SQL NULL).
	PubliclyAccessible bool
	PublicPort         int

	// Resources caps this database's memory and CPU, the same
	// *ServiceResources type and JSON-column storage
	// DesiredService.Resources already establishes (migrations/
	// 0028_desired_databases_resources.sql mirrors 0002's reasoning).
	// Unlike NodeID/ProjectID/the backup fields above, this is ordinary
	// desired state: SaveDesiredDatabase writes it on every save, the
	// same "whole record, not a special exception" treatment
	// DesiredService.Resources gets from SaveDesiredService.
	Resources *ServiceResources
}

// SaveDesiredDatabase creates or fully replaces the desired state for a
// named database, the same whole-record-replacement semantics as
// SaveDesiredService, including the identical NodeID exception
// SaveDesiredService's own doc comment explains.
func (db *DB) SaveDesiredDatabase(ctx context.Context, d DesiredDatabase) error {
	resourcesJSON, err := json.Marshal(d.Resources)
	if err != nil {
		return fmt.Errorf("store: marshal resources for database %q: %w", d.Name, err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO desired_databases (name, engine, version, resources, node_id, project_id, updated_at)
		VALUES (?, ?, ?, ?, '', NULL, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			engine = excluded.engine,
			version = excluded.version,
			resources = excluded.resources,
			updated_at = excluded.updated_at
	`, d.Name, d.Engine, d.Version, string(resourcesJSON))
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
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain, publicly_accessible, public_port, resources
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
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain, publicly_accessible, public_port, resources
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
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain, publicly_accessible, public_port, resources
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

// PublicPortRangeStart and PublicPortRangeEnd bound the host ports
// auto-assigned by SetDatabasePublicAccess when the caller doesn't
// request a specific one. Chosen clear of every port this control plane
// already binds itself (cmd/levelrail's defaultHTTPAddr ":8080",
// defaultAgentAddr ":9443") and of Caddy's 80/443, so an auto-assigned
// database port can never collide with the control plane's own
// listeners.
const (
	PublicPortRangeStart = 20000
	PublicPortRangeEnd   = 20999
)

// ErrPublicPortInUse is returned by SetDatabasePublicAccess when an
// explicitly requested port is already assigned to a different
// database. The unique index migrations/0026 adds on public_port is the
// hard backstop; this is the friendly error path that avoids ever
// reaching it in the normal case.
var ErrPublicPortInUse = errors.New("store: public port already in use by another database")

// ErrPublicPortRangeExhausted is returned by SetDatabasePublicAccess
// when auto-assignment finds no free port left in
// [PublicPortRangeStart, PublicPortRangeEnd].
var ErrPublicPortRangeExhausted = errors.New("store: no free public port available")

// SetDatabasePublicAccess is DesiredDatabase's counterpart to
// SetDatabaseBackupSchedule: its own endpoint
// (PUT/DELETE /api/v1/databases/{name}/public-access), its own store
// method, same separation-from-ordinary-update reasoning. Disabling
// (enabled=false) always clears public_port back to NULL regardless of
// requestedPort. Enabling with requestedPort 0 auto-assigns the lowest
// free port in [PublicPortRangeStart, PublicPortRangeEnd]; a non-zero
// requestedPort is used as-is if no other database already claims it.
// Returns the port actually assigned (0 when disabling).
//
// Runs inside a transaction so "is this port free" and "claim it" are
// atomic: db.SetMaxOpenConns(1) (store.go) already serializes every
// write against this same *sql.DB, so this is a correctness backstop
// against nothing racing it today, not a fix for an observed bug, the
// same reasoning claimServiceDomains (service.go) already applies to
// domain claims.
func (db *DB) SetDatabasePublicAccess(ctx context.Context, name string, enabled bool, requestedPort int) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: set public access for database %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	port := 0
	if enabled {
		port, err = claimPublicPort(ctx, tx, name, requestedPort)
		if err != nil {
			return 0, fmt.Errorf("store: set public access for database %q: %w", name, err)
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE desired_databases
		SET publicly_accessible = ?, public_port = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE name = ?
	`, enabled, sql.NullInt64{Int64: int64(port), Valid: enabled}, name)
	if err != nil {
		return 0, fmt.Errorf("store: set public access for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: set public access for database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return 0, ErrDatabaseNotFound
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: set public access for database %q: commit: %w", name, err)
	}
	return port, nil
}

// claimPublicPort resolves the port SetDatabasePublicAccess should write
// for name: requestedPort as-is once confirmed free, or the lowest free
// port in the configured range when requestedPort is 0. name is excluded
// from the "already in use" check so re-enabling with the same port (or
// re-saving) never conflicts with a database's own prior claim.
func claimPublicPort(ctx context.Context, tx *sql.Tx, name string, requestedPort int) (int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT public_port FROM desired_databases WHERE public_port IS NOT NULL AND name != ?
	`, name)
	if err != nil {
		return 0, fmt.Errorf("list claimed public ports: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	taken := make(map[int]bool)
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return 0, fmt.Errorf("scan claimed public port: %w", err)
		}
		taken[p] = true
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate claimed public ports: %w", err)
	}

	if requestedPort != 0 {
		if taken[requestedPort] {
			return 0, ErrPublicPortInUse
		}
		return requestedPort, nil
	}

	for p := PublicPortRangeStart; p <= PublicPortRangeEnd; p++ {
		if !taken[p] {
			return p, nil
		}
	}
	return 0, ErrPublicPortRangeExhausted
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
		SELECT name, engine, version, node_id, project_id, backup_target_id, backup_schedule, backup_retain, publicly_accessible, public_port, resources
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
		publiclyAccessible        bool
		publicPort                sql.NullInt64
		resourcesJSON             string
	)
	if err := scan(&d.Name, &d.Engine, &d.Version, &d.NodeID, &projectID, &backupTargetID, &d.BackupSchedule, &d.BackupRetain, &publiclyAccessible, &publicPort, &resourcesJSON); err != nil {
		return nil, err
	}
	d.ProjectID = projectID.String
	d.BackupTargetID = backupTargetID.String
	d.PubliclyAccessible = publiclyAccessible
	d.PublicPort = int(publicPort.Int64)
	if resourcesJSON != "null" {
		if err := json.Unmarshal([]byte(resourcesJSON), &d.Resources); err != nil {
			return nil, fmt.Errorf("unmarshal resources: %w", err)
		}
	}
	return &d, nil
}
