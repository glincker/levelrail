package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrMajorUpgradeNotFound is returned when an upgrade id matches no row.
var ErrMajorUpgradeNotFound = errors.New("store: database major upgrade not found")

// MajorUpgradeStatusRolledBack marks an upgrade undone by a rollback; the other
// statuses reuse the shared BackupStatus* values.
const MajorUpgradeStatusRolledBack = "rolled_back"

// MajorUpgrade is one guarded major version upgrade attempt of a database.
type MajorUpgrade struct {
	ID             string
	DatabaseName   string
	FromVersion    string
	ToVersion      string
	Status         string
	Phase          string
	SnapshotVolume string
	Error          string
	StartedAt      string
	FinishedAt     string
}

const majorUpgradeColumns = `id, database_name, from_version, to_version, status, phase, snapshot_volume, error, started_at, finished_at`

func majorUpgradeScanArgs(u *MajorUpgrade) []any {
	return []any{&u.ID, &u.DatabaseName, &u.FromVersion, &u.ToVersion, &u.Status, &u.Phase, &u.SnapshotVolume, &u.Error, &u.StartedAt, &u.FinishedAt}
}

// StartMajorUpgrade records an upgrade attempt as running.
func (db *DB) StartMajorUpgrade(ctx context.Context, u MajorUpgrade) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_major_upgrades (id, database_name, from_version, to_version, status, phase, started_at)
		VALUES (?, ?, ?, ?, ?, 'preflight', ?)
	`, u.ID, u.DatabaseName, u.FromVersion, u.ToVersion, BackupStatusRunning, u.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start major upgrade %q: %w", u.ID, err)
	}
	return nil
}

// UpdateMajorUpgradePhase records the phase an upgrade reached and, once one
// exists, its rollback snapshot volume (empty keeps the stored value).
func (db *DB) UpdateMajorUpgradePhase(ctx context.Context, id, phase, snapshotVolume string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE database_major_upgrades
		SET phase = ?, snapshot_volume = CASE WHEN ? = '' THEN snapshot_volume ELSE ? END
		WHERE id = ?
	`, phase, snapshotVolume, snapshotVolume, id)
	return checkMajorUpgradeUpdate(res, err, id, "update phase")
}

// FinishMajorUpgrade stores an upgrade's final status.
func (db *DB) FinishMajorUpgrade(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE database_major_upgrades SET status = ?, error = ?, finished_at = ? WHERE id = ?
	`, status, errMsg, finishedAt, id)
	return checkMajorUpgradeUpdate(res, err, id, "finish")
}

// ClearMajorUpgradeSnapshot forgets the rollback snapshot after it is deleted.
func (db *DB) ClearMajorUpgradeSnapshot(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `UPDATE database_major_upgrades SET snapshot_volume = '' WHERE id = ?`, id)
	return checkMajorUpgradeUpdate(res, err, id, "clear snapshot")
}

func checkMajorUpgradeUpdate(res sql.Result, err error, id, what string) error {
	if err != nil {
		return fmt.Errorf("store: %s major upgrade %q: %w", what, id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s major upgrade %q: %w", what, id, err)
	}
	if n == 0 {
		return ErrMajorUpgradeNotFound
	}
	return nil
}

// GetMajorUpgrade returns one upgrade attempt.
func (db *DB) GetMajorUpgrade(ctx context.Context, id string) (MajorUpgrade, error) {
	var u MajorUpgrade
	err := db.QueryRowContext(ctx, `SELECT `+majorUpgradeColumns+` FROM database_major_upgrades WHERE id = ?`, id).Scan(majorUpgradeScanArgs(&u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return MajorUpgrade{}, ErrMajorUpgradeNotFound
	}
	if err != nil {
		return MajorUpgrade{}, fmt.Errorf("store: get major upgrade %q: %w", id, err)
	}
	return u, nil
}

// ListMajorUpgrades returns a database's upgrade attempts, newest first.
func (db *DB) ListMajorUpgrades(ctx context.Context, databaseName string) ([]MajorUpgrade, error) {
	return db.queryMajorUpgrades(ctx, `WHERE database_name = ? ORDER BY started_at DESC`, databaseName)
}

// ListMajorUpgradeSnapshots returns every upgrade that still holds a rollback
// snapshot volume, across all databases.
func (db *DB) ListMajorUpgradeSnapshots(ctx context.Context) ([]MajorUpgrade, error) {
	return db.queryMajorUpgrades(ctx, `WHERE snapshot_volume != '' ORDER BY started_at DESC`)
}

// ListRunningMajorUpgrades returns upgrades still marked running, which after a
// control plane restart are the ones it interrupted.
func (db *DB) ListRunningMajorUpgrades(ctx context.Context) ([]MajorUpgrade, error) {
	return db.queryMajorUpgrades(ctx, `WHERE status = ? ORDER BY started_at`, BackupStatusRunning)
}

func (db *DB) queryMajorUpgrades(ctx context.Context, where string, args ...any) ([]MajorUpgrade, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+majorUpgradeColumns+` FROM database_major_upgrades `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list major upgrades: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MajorUpgrade
	for rows.Next() {
		var u MajorUpgrade
		if err := rows.Scan(majorUpgradeScanArgs(&u)...); err != nil {
			return nil, fmt.Errorf("store: scan major upgrade: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
