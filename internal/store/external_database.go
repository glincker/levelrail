package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Probe outcomes recorded on an ExternalDatabase.
const (
	ExternalHealthReachable   = "reachable"
	ExternalHealthSlow        = "slow"
	ExternalHealthAuthFailed  = "auth_failed"
	ExternalHealthTLSError    = "tls_error"
	ExternalHealthUnreachable = "unreachable"
	ExternalHealthUnknown     = "unknown"
)

// ErrExternalDatabaseNotFound is returned when no external database has that name.
var ErrExternalDatabaseNotFound = errors.New("store: external database not found")

// ErrExternalDatabaseExists is returned when the name is already taken.
var ErrExternalDatabaseExists = errors.New("store: external database already exists")

// ExternalDatabase is a database this platform connects to but does not run.
// The password is stored only through internal/secrets, never on this row.
type ExternalDatabase struct {
	Name            string
	Engine          string
	Host            string
	Port            int
	Username        string
	DatabaseName    string
	TLSMode         string
	Network         string
	NodeID          string
	ProjectID       string
	SourceContainer string
	HealthStatus    string
	HealthReason    string
	HealthLatencyMs int
	HealthCheckedAt string
	CreatedAt       string
	UpdatedAt       string
}

// ExternalDatabaseSecretsKey is the internal/secrets service name a record's password lives under.
func ExternalDatabaseSecretsKey(name string) string {
	return "external-database/" + name
}

// ExternalDatabasePasswordKey is the env key of that password.
const ExternalDatabasePasswordKey = "password"

const externalDatabaseColumns = `name, engine, host, port, username, database_name, tls_mode, network, node_id,
	COALESCE(project_id, ''), source_container, health_status, health_reason, health_latency_ms,
	health_checked_at, created_at, updated_at`

func scanExternalDatabase(row interface{ Scan(...any) error }) (ExternalDatabase, error) {
	var d ExternalDatabase
	err := row.Scan(&d.Name, &d.Engine, &d.Host, &d.Port, &d.Username, &d.DatabaseName, &d.TLSMode, &d.Network,
		&d.NodeID, &d.ProjectID, &d.SourceContainer, &d.HealthStatus, &d.HealthReason, &d.HealthLatencyMs,
		&d.HealthCheckedAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateExternalDatabase inserts d, failing with ErrExternalDatabaseExists on a name clash.
func (db *DB) CreateExternalDatabase(ctx context.Context, d ExternalDatabase) error {
	now := nowRFC3339()
	_, err := db.ExecContext(ctx, `
		INSERT INTO external_databases (name, engine, host, port, username, database_name, tls_mode, network,
			node_id, project_id, source_container, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.Name, d.Engine, d.Host, d.Port, d.Username, d.DatabaseName, d.TLSMode, d.Network, d.NodeID,
		sql.NullString{String: d.ProjectID, Valid: d.ProjectID != ""}, d.SourceContainer, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return ErrExternalDatabaseExists
		}
		return fmt.Errorf("store: create external database %q: %w", d.Name, err)
	}
	return nil
}

// UpdateExternalDatabase replaces the connection fields of an existing record.
func (db *DB) UpdateExternalDatabase(ctx context.Context, d ExternalDatabase) error {
	res, err := db.ExecContext(ctx, `
		UPDATE external_databases SET host = ?, port = ?, username = ?, database_name = ?, tls_mode = ?,
			network = ?, node_id = ?, updated_at = ?
		WHERE name = ?
	`, d.Host, d.Port, d.Username, d.DatabaseName, d.TLSMode, d.Network, d.NodeID, nowRFC3339(), d.Name)
	if err != nil {
		return fmt.Errorf("store: update external database %q: %w", d.Name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExternalDatabaseNotFound
	}
	return nil
}

// GetExternalDatabase returns the record or ErrExternalDatabaseNotFound.
func (db *DB) GetExternalDatabase(ctx context.Context, name string) (*ExternalDatabase, error) {
	d, err := scanExternalDatabase(db.QueryRowContext(ctx, `SELECT `+externalDatabaseColumns+` FROM external_databases WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExternalDatabaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get external database %q: %w", name, err)
	}
	return &d, nil
}

// ListExternalDatabases returns every record ordered by name.
func (db *DB) ListExternalDatabases(ctx context.Context) ([]ExternalDatabase, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+externalDatabaseColumns+` FROM external_databases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list external databases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ExternalDatabase
	for rows.Next() {
		d, err := scanExternalDatabase(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan external database: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteExternalDatabase removes only the local record. The remote database is never contacted.
func (db *DB) DeleteExternalDatabase(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM external_databases WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete external database %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExternalDatabaseNotFound
	}
	return nil
}

// SetExternalDatabaseHealth records the latest probe outcome.
func (db *DB) SetExternalDatabaseHealth(ctx context.Context, name, status, reason string, latencyMs int, checkedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE external_databases SET health_status = ?, health_reason = ?, health_latency_ms = ?, health_checked_at = ?
		WHERE name = ?
	`, status, reason, latencyMs, checkedAt.UTC().Format(time.RFC3339), name)
	if err != nil {
		return fmt.Errorf("store: set external database health %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExternalDatabaseNotFound
	}
	return nil
}
