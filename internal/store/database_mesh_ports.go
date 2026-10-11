package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrMeshPortsExhausted is returned when every port in the configured range is taken.
var ErrMeshPortsExhausted = errors.New("store: no free database mesh port in the configured range")

// GetDatabaseMeshPort returns a database's mesh port, 0 when none is allocated.
func (db *DB) GetDatabaseMeshPort(ctx context.Context, name string) (int, error) {
	var port int
	err := db.QueryRowContext(ctx, `SELECT port FROM database_mesh_ports WHERE database_name = ?`, name).Scan(&port)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: get mesh port for database %q: %w", name, err)
	}
	return port, nil
}

// EnsureDatabaseMeshPort returns the database's mesh port, allocating the
// lowest free one in [minPort, maxPort] on first use. Idempotent.
func (db *DB) EnsureDatabaseMeshPort(ctx context.Context, name string, minPort, maxPort int) (int, error) {
	if port, err := db.GetDatabaseMeshPort(ctx, name); err != nil || port != 0 {
		return port, err
	}
	if minPort <= 0 || maxPort < minPort || maxPort > 65535 {
		return 0, fmt.Errorf("store: invalid database mesh port range %d-%d", minPort, maxPort)
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO database_mesh_ports (database_name, port, created_at)
			SELECT ?, candidate, ? FROM (
				WITH RECURSIVE seq(candidate) AS (SELECT ? UNION ALL SELECT candidate + 1 FROM seq WHERE candidate < ?)
				SELECT candidate FROM seq WHERE candidate NOT IN (SELECT port FROM database_mesh_ports) LIMIT 1
			) WHERE true
			ON CONFLICT (database_name) DO NOTHING
		`, name, time.Now().UTC().Format(time.RFC3339Nano), minPort, maxPort)
		if err == nil || !isUniqueViolation(err) {
			break
		}
	}
	if err != nil {
		return 0, fmt.Errorf("store: allocate mesh port for database %q: %w", name, err)
	}
	port, err := db.GetDatabaseMeshPort(ctx, name)
	if err != nil {
		return 0, err
	}
	if port == 0 {
		return 0, ErrMeshPortsExhausted
	}
	return port, nil
}

// DeleteDatabaseMeshPort releases a database's mesh port.
func (db *DB) DeleteDatabaseMeshPort(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM database_mesh_ports WHERE database_name = ?`, name); err != nil {
		return fmt.Errorf("store: delete mesh port for database %q: %w", name, err)
	}
	return nil
}
