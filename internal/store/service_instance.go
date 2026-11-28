package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type legacyBackupsKey struct{}

// WithLegacyBackups lets volume backup listings include rows that no app instance owns.
func WithLegacyBackups(ctx context.Context) context.Context {
	return context.WithValue(ctx, legacyBackupsKey{}, true)
}

func legacyBackupsAllowed(ctx context.Context) bool {
	v, _ := ctx.Value(legacyBackupsKey{}).(bool)
	return v
}

// ServiceInstanceID returns the id of the app instance holding name, minting one the first time it is asked.
func (db *DB) ServiceInstanceID(ctx context.Context, name string) (string, error) {
	id, err := newDatabaseInstanceID()
	if err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO service_instances (name, instance_id) VALUES (?, ?)`, name, id); err != nil {
		return "", fmt.Errorf("store: record instance of service %q: %w", name, err)
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT instance_id FROM service_instances WHERE name = ?`, name).Scan(&got); err != nil {
		return "", fmt.Errorf("store: read instance of service %q: %w", name, err)
	}
	return got, nil
}

// OwnsVolumeBackup reports whether a volume backup row was taken by the current app of that name.
// An unstamped row belongs to no current app.
func (db *DB) OwnsVolumeBackup(ctx context.Context, serviceName, rowInstanceID string) (bool, error) {
	if rowInstanceID == "" {
		return false, nil
	}
	var got string
	err := db.QueryRowContext(ctx, `SELECT instance_id FROM service_instances WHERE name = ?`, serviceName).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: read instance of service %q: %w", serviceName, err)
	}
	return got == rowInstanceID, nil
}
