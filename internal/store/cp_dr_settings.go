package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CPDRSettings is the single control plane disaster recovery row. It holds
// age public recipients only, never a private key.
type CPDRSettings struct {
	InstallID         string
	Enabled           bool
	TargetID          string
	Recipients        []string
	Schedule          string
	RetainDaily       int
	RetainWeekly      int
	RetainMonthly     int
	DrillSchedule     string
	EscrowTargetID    string
	EscrowGeneratedAt time.Time
	EscrowAckedAt     time.Time
	LastBackupAt      time.Time
	LastBackupKey     string
	LastBackupError   string
	LastAttemptAt     time.Time
	LastDrillAt       time.Time
	LastDrillOK       bool
	LastDrillPartial  bool
	LastDrillDetail   string
	LastDrillMs       int64
}

const cpDRColumns = `install_id, enabled, target_id, recipients, schedule, retain_daily, retain_weekly, retain_monthly,
	drill_schedule, escrow_target_id, escrow_generated_at, escrow_acked_at, last_backup_at, last_backup_key,
	last_backup_error, last_attempt_at, last_drill_at, last_drill_ok, last_drill_partial, last_drill_detail, last_drill_ms`

func parseCPDRTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func formatCPDRTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// GetCPDRSettings returns the single settings row, which the migration seeds.
func (db *DB) GetCPDRSettings(ctx context.Context) (CPDRSettings, error) {
	var (
		s                                                    CPDRSettings
		recipients                                           string
		enabled, drillOK, drillPartial                       int
		escGen, escAck, lastBackup, lastAttempt, lastDrillAt string
	)
	err := db.QueryRowContext(ctx, `SELECT `+cpDRColumns+` FROM control_plane_dr_settings WHERE id = 1`).Scan(
		&s.InstallID, &enabled, &s.TargetID, &recipients, &s.Schedule, &s.RetainDaily, &s.RetainWeekly, &s.RetainMonthly,
		&s.DrillSchedule, &s.EscrowTargetID, &escGen, &escAck, &lastBackup, &s.LastBackupKey,
		&s.LastBackupError, &lastAttempt, &lastDrillAt, &drillOK, &drillPartial, &s.LastDrillDetail, &s.LastDrillMs)
	if err != nil {
		return CPDRSettings{}, fmt.Errorf("store: get control plane dr settings: %w", err)
	}
	if err := json.Unmarshal([]byte(recipients), &s.Recipients); err != nil {
		return CPDRSettings{}, fmt.Errorf("store: decode control plane dr recipients: %w", err)
	}
	s.Enabled, s.LastDrillOK, s.LastDrillPartial = enabled != 0, drillOK != 0, drillPartial != 0
	s.EscrowGeneratedAt, s.EscrowAckedAt = parseCPDRTime(escGen), parseCPDRTime(escAck)
	s.LastBackupAt, s.LastAttemptAt, s.LastDrillAt = parseCPDRTime(lastBackup), parseCPDRTime(lastAttempt), parseCPDRTime(lastDrillAt)
	return s, nil
}

// UpdateCPDRConfig replaces the operator-controlled fields only, so a
// settings save can never overwrite a concurrent run's recorded outcome.
func (db *DB) UpdateCPDRConfig(ctx context.Context, s CPDRSettings, now time.Time) error {
	recipients, err := json.Marshal(nonNilStrings(s.Recipients))
	if err != nil {
		return fmt.Errorf("store: encode control plane dr recipients: %w", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE control_plane_dr_settings SET enabled = ?, target_id = ?, recipients = ?, schedule = ?,
		retain_daily = ?, retain_weekly = ?, retain_monthly = ?, drill_schedule = ?, escrow_target_id = ?, updated_at = ? WHERE id = 1`,
		boolInt(s.Enabled), s.TargetID, string(recipients), s.Schedule, s.RetainDaily, s.RetainWeekly, s.RetainMonthly,
		s.DrillSchedule, s.EscrowTargetID, formatCPDRTime(now))
	if err != nil {
		return fmt.Errorf("store: update control plane dr config: %w", err)
	}
	return nil
}

// SetCPDRInstallID stores the install id once; an existing id is kept.
func (db *DB) SetCPDRInstallID(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `UPDATE control_plane_dr_settings SET install_id = ? WHERE id = 1 AND install_id = ''`, id); err != nil {
		return fmt.Errorf("store: set control plane install id: %w", err)
	}
	return nil
}

// RecordCPDRBackup records the outcome of one off-box backup attempt.
func (db *DB) RecordCPDRBackup(ctx context.Context, at time.Time, key, runErr string) error {
	q := `UPDATE control_plane_dr_settings SET last_attempt_at = ?, last_backup_error = ? WHERE id = 1`
	args := []any{formatCPDRTime(at), runErr}
	if runErr == "" {
		q = `UPDATE control_plane_dr_settings SET last_attempt_at = ?, last_backup_error = ?, last_backup_at = ?, last_backup_key = ? WHERE id = 1`
		args = append(args, formatCPDRTime(at), key)
	}
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("store: record control plane dr backup: %w", err)
	}
	return nil
}

// RecordCPDRDrill records the outcome of one restore drill.
func (db *DB) RecordCPDRDrill(ctx context.Context, at time.Time, ok, partial bool, detail string, ms int64) error {
	_, err := db.ExecContext(ctx, `UPDATE control_plane_dr_settings SET last_drill_at = ?, last_drill_ok = ?, last_drill_partial = ?,
		last_drill_detail = ?, last_drill_ms = ? WHERE id = 1`, formatCPDRTime(at), boolInt(ok), boolInt(partial), detail, ms)
	if err != nil {
		return fmt.Errorf("store: record control plane dr drill: %w", err)
	}
	return nil
}

// RecordCPDREscrow stamps escrow generation and clears any earlier acknowledgement.
func (db *DB) RecordCPDREscrow(ctx context.Context, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE control_plane_dr_settings SET escrow_generated_at = ?, escrow_acked_at = '' WHERE id = 1`, formatCPDRTime(at)); err != nil {
		return fmt.Errorf("store: record control plane dr escrow: %w", err)
	}
	return nil
}

// AckCPDREscrow records that the operator confirmed storing the escrow bundle offline.
func (db *DB) AckCPDREscrow(ctx context.Context, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE control_plane_dr_settings SET escrow_acked_at = ? WHERE id = 1 AND escrow_generated_at != ''`, formatCPDRTime(at)); err != nil {
		return fmt.Errorf("store: ack control plane dr escrow: %w", err)
	}
	return nil
}
