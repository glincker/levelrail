package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrVolumeBackupPolicyNotFound means no policy was ever set for the volume.
var ErrVolumeBackupPolicyNotFound = errors.New("store: volume backup policy not found")

// VolumeQuiescePause is the quiesce mode that pauses the app container.
const VolumeQuiescePause = "pause"

// VolumeBackupPolicy is a volume's grandfather-father-son retention counts
// and consistency hooks. A zero value means no extra policy.
type VolumeBackupPolicy struct {
	ServiceName   string
	VolumeName    string
	RetainDaily   int
	RetainWeekly  int
	RetainMonthly int
	PreHook       string
	PostHook      string
	Quiesce       string
	UpdatedAt     string
}

// HasRetention reports whether any grandfather-father-son count is set.
func (p VolumeBackupPolicy) HasRetention() bool {
	return p.RetainDaily > 0 || p.RetainWeekly > 0 || p.RetainMonthly > 0
}

// SetVolumeBackupPolicy upserts a volume's policy.
func (db *DB) SetVolumeBackupPolicy(ctx context.Context, p VolumeBackupPolicy) error {
	if p.UpdatedAt == "" {
		p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO volume_backup_policies (service_name, volume_name, retain_daily, retain_weekly, retain_monthly, pre_hook, post_hook, quiesce, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, volume_name) DO UPDATE SET
			retain_daily = excluded.retain_daily,
			retain_weekly = excluded.retain_weekly,
			retain_monthly = excluded.retain_monthly,
			pre_hook = excluded.pre_hook,
			post_hook = excluded.post_hook,
			quiesce = excluded.quiesce,
			updated_at = excluded.updated_at
	`, p.ServiceName, p.VolumeName, p.RetainDaily, p.RetainWeekly, p.RetainMonthly, p.PreHook, p.PostHook, p.Quiesce, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: set volume backup policy %q/%q: %w", p.ServiceName, p.VolumeName, err)
	}
	return nil
}

// GetVolumeBackupPolicy returns a volume's policy or ErrVolumeBackupPolicyNotFound.
func (db *DB) GetVolumeBackupPolicy(ctx context.Context, serviceName, volumeName string) (VolumeBackupPolicy, error) {
	var p VolumeBackupPolicy
	err := db.QueryRowContext(ctx, `
		SELECT service_name, volume_name, retain_daily, retain_weekly, retain_monthly, pre_hook, post_hook, quiesce, updated_at
		FROM volume_backup_policies WHERE service_name = ? AND volume_name = ?
	`, serviceName, volumeName).Scan(&p.ServiceName, &p.VolumeName, &p.RetainDaily, &p.RetainWeekly, &p.RetainMonthly, &p.PreHook, &p.PostHook, &p.Quiesce, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return VolumeBackupPolicy{}, ErrVolumeBackupPolicyNotFound
	}
	if err != nil {
		return VolumeBackupPolicy{}, fmt.Errorf("store: get volume backup policy %q/%q: %w", serviceName, volumeName, err)
	}
	return p, nil
}

// SetBackupSealInfo records how a succeeded backup object was encoded and its
// content manifest.
func (db *DB) SetBackupSealInfo(ctx context.Context, id, codec, manifestDigest string, files, bytes int64) error {
	res, err := db.ExecContext(ctx, `
		UPDATE backup_history SET codec = ?, manifest_digest = ?, manifest_files = ?, manifest_bytes = ?
		WHERE id = ?
	`, codec, manifestDigest, files, bytes, id)
	if err != nil {
		return fmt.Errorf("store: set backup seal info %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBackupHistoryNotFound
	}
	return nil
}

const backupHistoryColumns = `id, database_name, resource_kind, service_name, volume_name, target_id, object_key, size_bytes, status, error, started_at, finished_at, checksum_sha256, codec, manifest_digest, manifest_files, manifest_bytes`

// ListSucceededVolumeBackups returns every succeeded backup of one volume,
// newest first, with no paging: retention needs the whole set.
func (db *DB) ListSucceededVolumeBackups(ctx context.Context, serviceName, volumeName string) ([]BackupHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+backupHistoryColumns+`
		FROM backup_history
		WHERE resource_kind = ? AND service_name = ? AND volume_name = ? AND status = ?
		ORDER BY started_at DESC
	`, BackupResourceKindVolume, serviceName, volumeName, BackupStatusSucceeded)
	if err != nil {
		return nil, fmt.Errorf("store: list succeeded backups for %q/%q: %w", serviceName, volumeName, err)
	}
	defer func() { _ = rows.Close() }()
	return scanBackupHistoryRows(rows)
}

// ListRunningBackups returns backup rows still marked running that started
// before cutoff: attempts the control plane lost track of.
func (db *DB) ListRunningBackups(ctx context.Context, startedBefore time.Time) ([]BackupHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+backupHistoryColumns+`
		FROM backup_history WHERE status = ? AND started_at < ? ORDER BY started_at
	`, BackupStatusRunning, startedBefore.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: list running backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanBackupHistoryRows(rows)
}

// ListSucceededBackups returns every succeeded backup newest first, for the
// per-resource health rollup.
func (db *DB) ListSucceededBackups(ctx context.Context) ([]BackupHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+backupHistoryColumns+`
		FROM backup_history WHERE status = ? ORDER BY started_at DESC
	`, BackupStatusSucceeded)
	if err != nil {
		return nil, fmt.Errorf("store: list succeeded backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanBackupHistoryRows(rows)
}

// Drill statuses and triggers.
const (
	DrillStatusRunning = "running"
	DrillStatusPassed  = "passed"
	DrillStatusFailed  = "failed"

	DrillTriggerManual    = "manual"
	DrillTriggerScheduled = "scheduled"
)

// BackupDrill is one restore drill attempt.
type BackupDrill struct {
	ID              string
	BackupHistoryID string
	ResourceKind    string
	DatabaseName    string
	ServiceName     string
	VolumeName      string
	TargetID        string
	Trigger         string
	Status          string
	Stage           string
	ObjectOK        bool
	ChecksumOK      bool
	RestoreOK       bool
	ContentOK       bool
	Files           int64
	Bytes           int64
	DurationMS      int64
	Error           string
	StartedAt       string
	FinishedAt      string
}

// StartBackupDrill writes a running drill row.
func (db *DB) StartBackupDrill(ctx context.Context, d BackupDrill) error {
	if d.Trigger == "" {
		d.Trigger = DrillTriggerManual
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_drills (id, backup_history_id, resource_kind, database_name, service_name, volume_name, target_id, trigger, status, stage, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'start', ?)
	`, d.ID, d.BackupHistoryID, d.ResourceKind, d.DatabaseName, d.ServiceName, d.VolumeName, d.TargetID, d.Trigger, DrillStatusRunning, d.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start backup drill %q: %w", d.ID, err)
	}
	return nil
}

// FinishBackupDrill records a drill's final outcome.
func (db *DB) FinishBackupDrill(ctx context.Context, d BackupDrill) error {
	res, err := db.ExecContext(ctx, `
		UPDATE backup_drills SET status = ?, stage = ?, object_ok = ?, checksum_ok = ?, restore_ok = ?, content_ok = ?,
			files = ?, bytes = ?, duration_ms = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, d.Status, d.Stage, d.ObjectOK, d.ChecksumOK, d.RestoreOK, d.ContentOK, d.Files, d.Bytes, d.DurationMS, d.Error, d.FinishedAt, d.ID)
	if err != nil {
		return fmt.Errorf("store: finish backup drill %q: %w", d.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: finish backup drill %q: not found", d.ID)
	}
	return nil
}

const backupDrillColumns = `id, backup_history_id, resource_kind, database_name, service_name, volume_name, target_id, trigger, status, stage, object_ok, checksum_ok, restore_ok, content_ok, files, bytes, duration_ms, error, started_at, finished_at`

func scanBackupDrills(rows *sql.Rows) ([]BackupDrill, error) {
	var out []BackupDrill
	for rows.Next() {
		var d BackupDrill
		if err := rows.Scan(&d.ID, &d.BackupHistoryID, &d.ResourceKind, &d.DatabaseName, &d.ServiceName, &d.VolumeName, &d.TargetID, &d.Trigger, &d.Status, &d.Stage, &d.ObjectOK, &d.ChecksumOK, &d.RestoreOK, &d.ContentOK, &d.Files, &d.Bytes, &d.DurationMS, &d.Error, &d.StartedAt, &d.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan backup drill: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetBackupDrill returns one drill or a wrapped sql.ErrNoRows.
func (db *DB) GetBackupDrill(ctx context.Context, id string) (BackupDrill, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+backupDrillColumns+` FROM backup_drills WHERE id = ?`, id)
	if err != nil {
		return BackupDrill{}, fmt.Errorf("store: get backup drill %q: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	out, err := scanBackupDrills(rows)
	if err != nil {
		return BackupDrill{}, fmt.Errorf("store: get backup drill %q: %w", id, err)
	}
	if len(out) == 0 {
		return BackupDrill{}, fmt.Errorf("store: get backup drill %q: %w", id, sql.ErrNoRows)
	}
	return out[0], nil
}

// ListBackupDrills returns drills newest first. serviceName, volumeName and
// databaseName narrow to one resource when non-empty.
func (db *DB) ListBackupDrills(ctx context.Context, serviceName, volumeName, databaseName string, limit int) ([]BackupDrill, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+backupDrillColumns+` FROM backup_drills
		WHERE (? = '' OR service_name = ?) AND (? = '' OR volume_name = ?) AND (? = '' OR database_name = ?)
		ORDER BY started_at DESC LIMIT ?
	`, serviceName, serviceName, volumeName, volumeName, databaseName, databaseName, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list backup drills: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanBackupDrills(rows)
}

// BackupTargetProtection is the last bucket protection probe of a target.
type BackupTargetProtection struct {
	TargetID   string
	ObjectLock bool
	LockMode   string
	Versioning string
	CanDelete  bool
	ProbeError string
	CheckedAt  string
}

// SetBackupTargetProtection upserts a probe result.
func (db *DB) SetBackupTargetProtection(ctx context.Context, p BackupTargetProtection) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_target_protection (target_id, object_lock, lock_mode, versioning, can_delete, probe_error, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (target_id) DO UPDATE SET object_lock = excluded.object_lock, lock_mode = excluded.lock_mode,
			versioning = excluded.versioning, can_delete = excluded.can_delete, probe_error = excluded.probe_error, checked_at = excluded.checked_at
	`, p.TargetID, p.ObjectLock, p.LockMode, p.Versioning, p.CanDelete, p.ProbeError, p.CheckedAt)
	if err != nil {
		return fmt.Errorf("store: set backup target protection %q: %w", p.TargetID, err)
	}
	return nil
}

// ListBackupTargetProtection returns every stored probe.
func (db *DB) ListBackupTargetProtection(ctx context.Context) ([]BackupTargetProtection, error) {
	rows, err := db.QueryContext(ctx, `SELECT target_id, object_lock, lock_mode, versioning, can_delete, probe_error, checked_at FROM backup_target_protection`)
	if err != nil {
		return nil, fmt.Errorf("store: list backup target protection: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BackupTargetProtection
	for rows.Next() {
		var p BackupTargetProtection
		if err := rows.Scan(&p.TargetID, &p.ObjectLock, &p.LockMode, &p.Versioning, &p.CanDelete, &p.ProbeError, &p.CheckedAt); err != nil {
			return nil, fmt.Errorf("store: scan backup target protection: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
