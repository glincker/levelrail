package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SelfUpgradeAttempt is one recorded host-run self-upgrade (migrations/0456).
// StepsJSON and AckedJSON are opaque to the store.
type SelfUpgradeAttempt struct {
	ID          string
	FromVersion string
	ToVersion   string
	FromSchema  int
	ToSchema    int
	Initiator   string
	Outcome     string
	FailedStep  string
	Error       string
	BackupName  string
	AckedJSON   string
	StepsJSON   string
	StartedAt   string
	FinishedAt  string
}

const selfUpgradeColumns = `id, from_version, to_version, from_schema, to_schema, initiator, outcome,
	failed_step, error, backup_name, acked_json, steps_json, started_at, finished_at`

// UpsertSelfUpgradeAttempt stores a, replacing an earlier copy of the same id,
// so importing a journal twice is harmless.
func (db *DB) UpsertSelfUpgradeAttempt(ctx context.Context, a SelfUpgradeAttempt) error {
	if a.AckedJSON == "" {
		a.AckedJSON = "[]"
	}
	if a.StepsJSON == "" {
		a.StepsJSON = "[]"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO self_upgrade_attempts (`+selfUpgradeColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			from_version = excluded.from_version, to_version = excluded.to_version,
			from_schema = excluded.from_schema, to_schema = excluded.to_schema,
			initiator = excluded.initiator, outcome = excluded.outcome,
			failed_step = excluded.failed_step, error = excluded.error,
			backup_name = excluded.backup_name, acked_json = excluded.acked_json,
			steps_json = excluded.steps_json, finished_at = excluded.finished_at`,
		a.ID, a.FromVersion, a.ToVersion, a.FromSchema, a.ToSchema, a.Initiator, a.Outcome,
		a.FailedStep, a.Error, a.BackupName, a.AckedJSON, a.StepsJSON, a.StartedAt, a.FinishedAt)
	if err != nil {
		return fmt.Errorf("store self upgrade attempt %s: %w", a.ID, err)
	}
	return nil
}

// ListSelfUpgradeAttempts returns the newest attempts first.
func (db *DB) ListSelfUpgradeAttempts(ctx context.Context, limit int) ([]SelfUpgradeAttempt, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT `+selfUpgradeColumns+` FROM self_upgrade_attempts ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list self upgrade attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SelfUpgradeAttempt
	for rows.Next() {
		a, err := scanSelfUpgrade(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanSelfUpgrade(rows *sql.Rows) (SelfUpgradeAttempt, error) {
	var a SelfUpgradeAttempt
	err := rows.Scan(&a.ID, &a.FromVersion, &a.ToVersion, &a.FromSchema, &a.ToSchema, &a.Initiator, &a.Outcome,
		&a.FailedStep, &a.Error, &a.BackupName, &a.AckedJSON, &a.StepsJSON, &a.StartedAt, &a.FinishedAt)
	if err != nil {
		return SelfUpgradeAttempt{}, fmt.Errorf("scan self upgrade attempt: %w", err)
	}
	return a, nil
}
