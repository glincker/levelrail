package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MeshSetting is the persisted mesh on/off choice. Set is false when the
// operator has never chosen.
type MeshSetting struct {
	Enabled   bool
	Set       bool
	UpdatedAt time.Time
	UpdatedBy string
}

// GetMeshSetting returns the persisted mesh choice, or the zero value when none exists.
func (db *DB) GetMeshSetting(ctx context.Context) (MeshSetting, error) {
	var (
		enabled   int
		updatedAt string
		updatedBy string
	)
	err := db.QueryRowContext(ctx, `SELECT enabled, updated_at, updated_by FROM mesh_settings WHERE id = 1`).
		Scan(&enabled, &updatedAt, &updatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return MeshSetting{}, nil
	}
	if err != nil {
		return MeshSetting{}, fmt.Errorf("store: get mesh setting: %w", err)
	}
	t, _ := time.Parse(time.RFC3339Nano, updatedAt)
	return MeshSetting{Enabled: enabled != 0, Set: true, UpdatedAt: t, UpdatedBy: updatedBy}, nil
}

// SetMeshSetting persists the mesh choice.
func (db *DB) SetMeshSetting(ctx context.Context, enabled bool, actor string, now time.Time) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO mesh_settings (id, enabled, updated_at, updated_by) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		v, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set mesh setting: %w", err)
	}
	return nil
}
