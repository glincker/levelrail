package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BackupTargetInUseError lists the live things that still use a target.
type BackupTargetInUseError struct {
	Blockers []string
}

func (e *BackupTargetInUseError) Error() string {
	return "store: backup target is in use by " + strings.Join(e.Blockers, "; ")
}

// RetireBackupTarget removes a target from every live view when nothing live
// uses it. History rows of deleted resources stay for audit and keep their
// target id, so they still render; bucket objects are never touched. It
// returns how many history rows were detached and how many failed attempts of
// deleted resources were pruned.
func (db *DB) RetireBackupTarget(ctx context.Context, id string) (RetireResult, error) {
	var res RetireResult
	detached, pruned, err := db.retireBackupTarget(ctx, id)
	res.Detached, res.Pruned = detached, pruned
	if err != nil {
		return RetireResult{}, err
	}
	// A retired target cannot come back, so its stored credentials go with it.
	if err := db.DeleteServiceSecrets(ctx, BackupTargetSecretsKey(id)); err != nil {
		return res, fmt.Errorf("store: wipe credentials of backup target %q: %w", id, err)
	}
	return res, nil
}

// RetireResult reports what a retire changed besides the target row.
type RetireResult struct {
	Detached int
	Pruned   int
}

func (db *DB) retireBackupTarget(ctx context.Context, id string) (int, int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("store: retire backup target %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_targets WHERE id = ? AND deleted_at = ''`, id).Scan(&exists); err != nil {
		return 0, 0, fmt.Errorf("store: retire backup target %q: %w", id, err)
	}
	if exists == 0 {
		return 0, 0, ErrBackupTargetNotFound
	}

	// Failed attempts left no artifact, so they never pin a target.
	pr, err := tx.ExecContext(ctx, `
		DELETE FROM backup_history
		WHERE target_id = ? AND status = ?
		  AND NOT EXISTS (SELECT 1 FROM desired_databases d WHERE backup_history.resource_kind = 'database' AND d.name = backup_history.database_name)
		  AND NOT EXISTS (SELECT 1 FROM desired_services sv WHERE backup_history.resource_kind = 'volume' AND sv.name = backup_history.service_name)
		  AND NOT EXISTS (SELECT 1 FROM deploy_db_snapshots p WHERE p.backup_history_id = backup_history.id)
		  AND NOT EXISTS (SELECT 1 FROM restore_history r WHERE r.backup_history_id = backup_history.id)
		  AND NOT EXISTS (SELECT 1 FROM backup_verification v WHERE v.backup_history_id = backup_history.id)
		  AND NOT EXISTS (SELECT 1 FROM volume_clone_restores c WHERE c.backup_history_id = backup_history.id)`,
		id, BackupStatusFailed)
	if err != nil {
		return 0, 0, fmt.Errorf("store: prune failed backup history for target %q: %w", id, err)
	}
	pruned64, _ := pr.RowsAffected()
	pruned := int(pruned64)

	blockers, err := backupTargetBlockers(ctx, tx, id)
	if err != nil {
		return 0, 0, err
	}
	if len(blockers) > 0 {
		return 0, 0, &BackupTargetInUseError{Blockers: blockers}
	}

	var detached int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_history WHERE target_id = ?`, id).Scan(&detached); err != nil {
		return 0, 0, fmt.Errorf("store: count detached history for target %q: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM storage_destination_options WHERE target_id = ?`, id); err != nil {
		return 0, 0, fmt.Errorf("store: retire backup target %q: %w", id, err)
	}
	// With no history left the row can go entirely.
	if detached == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM backup_targets WHERE id = ?`, id); err != nil {
			return 0, 0, fmt.Errorf("store: delete backup target %q: %w", id, err)
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE backup_targets SET deleted_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return 0, 0, fmt.Errorf("store: retire backup target %q: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("store: retire backup target %q: %w", id, err)
	}
	return detached, pruned, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// backupTargetBlockers names each live reference: schedules, policies,
// running work, and history that belongs to a resource that still exists.
func backupTargetBlockers(ctx context.Context, q queryer, id string) ([]string, error) {
	checks := []struct{ label, query string }{
		{"database backup schedule", `SELECT name FROM desired_databases WHERE backup_target_id = ?`},
		{"volume backup schedule", `SELECT service_name || '/' || volume_name FROM service_volume_backups WHERE backup_target_id = ?`},
		{"app storage target", `SELECT name FROM desired_services WHERE storage_target_id = ?`},
		{"log archive policy", `SELECT CASE WHEN app_name = '' THEN 'global' ELSE app_name END FROM log_archive_policies WHERE target_id = ?`},
		{"storage bucket", `SELECT name FROM storage_buckets WHERE target_id = ?`},
		{"control plane DR backup", `SELECT 'settings' FROM control_plane_dr_settings WHERE target_id = ? OR escrow_target_id = ?`},
		{"backup copy destination", `SELECT 'settings' FROM backup_copy_settings WHERE target_id = ?`},
		{"queued backup copy", `SELECT backup_history_id FROM backup_copies WHERE target_id = ? AND status IN ('pending', 'copying')`},
		{"predeploy snapshot setting", `SELECT app FROM app_predeploy_snapshot_settings WHERE target_id = ?`},
		{"running base backup", `SELECT database_name FROM base_backup_history WHERE target_id = ? AND status = 'running'`},
		{"running backup drill", `SELECT backup_history_id FROM backup_drills WHERE target_id = ? AND status = 'running'`},
		{"build cache", `SELECT app_name FROM build_cache_settings WHERE target_id = ?`},
		{"storage default", `SELECT data_class FROM storage_defaults WHERE target_id = ?`},
		{"running backup", `SELECT COALESCE(NULLIF(database_name, ''), service_name) FROM backup_history WHERE target_id = ? AND status = 'running'`},
		{"pending restore", `SELECT r.database_name FROM restore_history r JOIN backup_history h ON h.id = r.backup_history_id WHERE h.target_id = ? AND r.status = 'running'`},
		{"pending volume restore", `SELECT h.service_name FROM volume_clone_restores c JOIN backup_history h ON h.id = c.backup_history_id WHERE h.target_id = ? AND c.status = 'running'`},
		{"backup history of existing database", `SELECT DISTINCT h.database_name FROM backup_history h JOIN desired_databases d ON d.name = h.database_name WHERE h.target_id = ? AND h.resource_kind = 'database'`},
		{"backup history of existing app", `SELECT DISTINCT h.service_name FROM backup_history h JOIN desired_services s ON s.name = h.service_name WHERE h.target_id = ? AND h.resource_kind = 'volume'`},
	}
	var out []string
	for _, c := range checks {
		args := make([]any, strings.Count(c.query, "?"))
		for i := range args {
			args[i] = id
		}
		rows, err := q.QueryContext(ctx, c.query, args...)
		if err != nil {
			return nil, fmt.Errorf("store: check %s for target %q: %w", c.label, id, err)
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("store: scan %s for target %q: %w", c.label, id, err)
			}
			names = append(names, n)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, fmt.Errorf("store: check %s for target %q: %w", c.label, id, err)
		}
		if len(names) > 0 {
			out = append(out, fmt.Sprintf("%d %s (%s)", len(names), c.label, strings.Join(names, ", ")))
		}
	}
	return out, nil
}
