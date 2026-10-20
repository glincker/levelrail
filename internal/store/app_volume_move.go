package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrAppVolumeMoveNotFound is returned by UpdateAppVolumeMoveSteps,
// FinishAppVolumeMove, and GetAppVolumeMove when id doesn't match any row.
var ErrAppVolumeMoveNotFound = errors.New("store: app volume move record not found")

// AppVolumeMoveStep is one step of a "move with volumes" attempt (stop the
// app, archive one volume, restore it onto the new node, ...), appended to
// AppVolumeMove.Steps as each step starts and finishes: this is what makes
// a half-completed move diagnosable rather than a black box, see
// internal/api's handleMoveAppWithVolumes for the exact step sequence.
type AppVolumeMoveStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// AppVolumeMove is one "move this app to another node, taking its volumes
// with it" attempt: ServiceName's placement moving from FromNodeID to
// ToNodeID, with Steps recording exactly how far it got. Status mirrors
// BackupHistory's own running/succeeded/failed lifecycle.
type AppVolumeMove struct {
	ID          string
	ServiceName string
	FromNodeID  string
	ToNodeID    string
	Status      string
	Error       string
	Steps       []AppVolumeMoveStep
	StartedAt   string
	FinishedAt  string
}

// StartAppVolumeMove records a move attempt beginning, status
// BackupStatusRunning with an empty step list, mirroring
// StartVolumeCloneRestore's own shape.
func (db *DB) StartAppVolumeMove(ctx context.Context, m AppVolumeMove) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_volume_moves (id, service_name, from_node_id, to_node_id, status, error, steps, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, '', '[]', ?, '')
	`, m.ID, m.ServiceName, m.FromNodeID, m.ToNodeID, BackupStatusRunning, m.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start app volume move %q: %w", m.ID, err)
	}
	return nil
}

// UpdateAppVolumeMoveSteps overwrites a running move's step list: called
// after every step starts and again after it finishes, so a caller polling
// GetAppVolumeMove mid-flight sees real progress, not just a final result.
func (db *DB) UpdateAppVolumeMoveSteps(ctx context.Context, id string, steps []AppVolumeMoveStep) error {
	if steps == nil {
		steps = []AppVolumeMoveStep{}
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("store: marshal app volume move steps %q: %w", id, err)
	}
	res, err := db.ExecContext(ctx, `UPDATE app_volume_moves SET steps = ? WHERE id = ?`, string(stepsJSON), id)
	if err != nil {
		return fmt.Errorf("store: update app volume move steps %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update app volume move steps %q: %w", id, err)
	}
	if n == 0 {
		return ErrAppVolumeMoveNotFound
	}
	return nil
}

// FinishAppVolumeMove updates a running move row to its final status,
// mirroring FinishVolumeCloneRestore exactly.
func (db *DB) FinishAppVolumeMove(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE app_volume_moves
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish app volume move %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish app volume move %q: %w", id, err)
	}
	if n == 0 {
		return ErrAppVolumeMoveNotFound
	}
	return nil
}

// GetAppVolumeMove returns one move attempt by id, its full step history
// included.
func (db *DB) GetAppVolumeMove(ctx context.Context, id string) (AppVolumeMove, error) {
	var m AppVolumeMove
	var stepsJSON string
	err := db.QueryRowContext(ctx, `
		SELECT id, service_name, from_node_id, to_node_id, status, error, steps, started_at, finished_at
		FROM app_volume_moves
		WHERE id = ?
	`, id).Scan(&m.ID, &m.ServiceName, &m.FromNodeID, &m.ToNodeID, &m.Status, &m.Error, &stepsJSON, &m.StartedAt, &m.FinishedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AppVolumeMove{}, ErrAppVolumeMoveNotFound
		}
		return AppVolumeMove{}, fmt.Errorf("store: get app volume move %q: %w", id, err)
	}
	if err := json.Unmarshal([]byte(stepsJSON), &m.Steps); err != nil {
		return AppVolumeMove{}, fmt.Errorf("store: unmarshal app volume move steps %q: %w", id, err)
	}
	return m, nil
}

// ListAppVolumeMoves returns every move attempt for serviceName, newest
// first, mirroring ListVolumeCloneRestores' own shape.
func (db *DB) ListAppVolumeMoves(ctx context.Context, serviceName string) ([]AppVolumeMove, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, from_node_id, to_node_id, status, error, steps, started_at, finished_at
		FROM app_volume_moves
		WHERE service_name = ?
		ORDER BY started_at DESC
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list app volume moves for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []AppVolumeMove
	for rows.Next() {
		var m AppVolumeMove
		var stepsJSON string
		if err := rows.Scan(&m.ID, &m.ServiceName, &m.FromNodeID, &m.ToNodeID, &m.Status, &m.Error, &stepsJSON, &m.StartedAt, &m.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan app volume move row: %w", err)
		}
		if err := json.Unmarshal([]byte(stepsJSON), &m.Steps); err != nil {
			return nil, fmt.Errorf("store: unmarshal app volume move steps %q: %w", m.ID, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app volume move rows: %w", err)
	}
	return out, nil
}
