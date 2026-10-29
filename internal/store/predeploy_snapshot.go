package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Pre-deploy snapshot modes: off skips, on is best effort, require blocks
// the deploy when the snapshot fails.
const (
	PredeployModeOff     = "off"
	PredeployModeOn      = "on"
	PredeployModeRequire = "require"
)

// Snapshot and restore outcomes recorded on a deploy_db_snapshots row.
const (
	PredeploySnapshotSucceeded = "succeeded"
	PredeploySnapshotFailed    = "failed"
	PredeployRestoreRunning    = "running"
	PredeployRestoreSucceeded  = "succeeded"
	PredeployRestoreFailed     = "failed"
)

// ErrPredeploySnapshotNotFound is returned when no snapshot row matches.
var ErrPredeploySnapshotNotFound = errors.New("store: predeploy snapshot not found")

// PredeploySnapshotSettings is one app's snapshot policy.
type PredeploySnapshotSettings struct {
	Mode     string
	TargetID string
}

// PredeploySnapshot is one database snapshot taken ahead of a deploy.
type PredeploySnapshot struct {
	ID               string
	App              string
	DeployID         string
	PriorDeployID    string
	DatabaseName     string
	Engine           string
	BackupHistoryID  string
	Status           string
	Error            string
	SizeBytes        int64
	CreatedAt        string
	RestoredDatabase string
	RestoreStatus    string
	RestoreError     string
}

// GetPredeploySnapshotSettings returns the app's policy, off when never set.
func (db *DB) GetPredeploySnapshotSettings(ctx context.Context, app string) (PredeploySnapshotSettings, error) {
	var s PredeploySnapshotSettings
	err := db.QueryRowContext(ctx, `SELECT mode, target_id FROM app_predeploy_snapshot_settings WHERE app = ?`, app).Scan(&s.Mode, &s.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return PredeploySnapshotSettings{Mode: PredeployModeOff}, nil
	}
	if err != nil {
		return PredeploySnapshotSettings{}, fmt.Errorf("store: get predeploy snapshot settings for %q: %w", app, err)
	}
	return s, nil
}

// SetPredeploySnapshotSettings upserts the app's policy.
func (db *DB) SetPredeploySnapshotSettings(ctx context.Context, app string, s PredeploySnapshotSettings) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_predeploy_snapshot_settings (app, mode, target_id) VALUES (?, ?, ?)
		ON CONFLICT(app) DO UPDATE SET mode = excluded.mode, target_id = excluded.target_id
	`, app, s.Mode, s.TargetID)
	if err != nil {
		return fmt.Errorf("store: set predeploy snapshot settings for %q: %w", app, err)
	}
	return nil
}

// SavePredeploySnapshot inserts a snapshot row.
func (db *DB) SavePredeploySnapshot(ctx context.Context, p PredeploySnapshot) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_db_snapshots (id, app, deploy_id, prior_deploy_id, database_name, engine, backup_history_id, status, error, size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.App, p.DeployID, p.PriorDeployID, p.DatabaseName, p.Engine, p.BackupHistoryID, p.Status, p.Error, p.SizeBytes, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save predeploy snapshot %q: %w", p.ID, err)
	}
	return nil
}

// AttachPredeploySnapshotsToDeploy records the deploy the given snapshots preceded.
func (db *DB) AttachPredeploySnapshotsToDeploy(ctx context.Context, ids []string, deployID string) error {
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `UPDATE deploy_db_snapshots SET deploy_id = ? WHERE id = ?`, deployID, id); err != nil {
			return fmt.Errorf("store: attach predeploy snapshot %q to deploy: %w", id, err)
		}
	}
	return nil
}

// SetPredeploySnapshotRestore records rollback-with-data progress on a snapshot.
func (db *DB) SetPredeploySnapshotRestore(ctx context.Context, id, restoredDatabase, status, errMsg string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_db_snapshots SET restored_database = ?, restore_status = ?, restore_error = ? WHERE id = ?
	`, restoredDatabase, status, errMsg, id)
	if err != nil {
		return fmt.Errorf("store: set predeploy snapshot restore %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPredeploySnapshotNotFound
	}
	return nil
}

const predeploySnapshotColumns = `id, app, deploy_id, prior_deploy_id, database_name, engine, backup_history_id, status, error, size_bytes, created_at, restored_database, restore_status, restore_error`

func (db *DB) queryPredeploySnapshots(ctx context.Context, where string, args ...any) ([]PredeploySnapshot, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+predeploySnapshotColumns+` FROM deploy_db_snapshots WHERE `+where+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list predeploy snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PredeploySnapshot
	for rows.Next() {
		var p PredeploySnapshot
		if err := rows.Scan(&p.ID, &p.App, &p.DeployID, &p.PriorDeployID, &p.DatabaseName, &p.Engine, &p.BackupHistoryID, &p.Status, &p.Error, &p.SizeBytes, &p.CreatedAt, &p.RestoredDatabase, &p.RestoreStatus, &p.RestoreError); err != nil {
			return nil, fmt.Errorf("store: scan predeploy snapshot: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPredeploySnapshots returns the app's snapshots, newest first.
func (db *DB) ListPredeploySnapshots(ctx context.Context, app string) ([]PredeploySnapshot, error) {
	return db.queryPredeploySnapshots(ctx, `app = ?`, app)
}

// ListPredeploySnapshotsForDeploy returns the snapshots taken before one deploy.
func (db *DB) ListPredeploySnapshotsForDeploy(ctx context.Context, app, deployID string) ([]PredeploySnapshot, error) {
	return db.queryPredeploySnapshots(ctx, `app = ? AND deploy_id = ?`, app, deployID)
}

// DeletePredeploySnapshot removes a snapshot row.
func (db *DB) DeletePredeploySnapshot(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_db_snapshots WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete predeploy snapshot %q: %w", id, err)
	}
	return nil
}
