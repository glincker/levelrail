package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrMigrationVerifyNotFound is returned when no parity report has been stored yet.
var ErrMigrationVerifyNotFound = errors.New("migration verify report not found")

// MigrationVerifyReport is the latest stored parity report: the JSON body
// plus the columns the list views need (migrations/2920_migration_verify_report.sql).
type MigrationVerifyReport struct {
	Platform    string
	GeneratedAt time.Time
	Verdict     string
	Body        []byte
}

// SaveMigrationVerifyReport replaces the stored report.
func (db *DB) SaveMigrationVerifyReport(ctx context.Context, r MigrationVerifyReport) error {
	_, err := db.ExecContext(ctx, `INSERT INTO migration_verify_report (id, platform, generated_at, verdict, report)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET platform = excluded.platform, generated_at = excluded.generated_at, verdict = excluded.verdict, report = excluded.report`,
		r.Platform, r.GeneratedAt.UTC().Format(time.RFC3339Nano), r.Verdict, string(r.Body))
	if err != nil {
		return fmt.Errorf("store: save migration verify report: %w", err)
	}
	return nil
}

// GetMigrationVerifyReport returns the latest stored report.
func (db *DB) GetMigrationVerifyReport(ctx context.Context) (MigrationVerifyReport, error) {
	var (
		r    MigrationVerifyReport
		at   string
		body string
	)
	err := db.QueryRowContext(ctx, `SELECT platform, generated_at, verdict, report FROM migration_verify_report WHERE id = 1`).Scan(&r.Platform, &at, &r.Verdict, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return MigrationVerifyReport{}, ErrMigrationVerifyNotFound
	}
	if err != nil {
		return MigrationVerifyReport{}, fmt.Errorf("store: get migration verify report: %w", err)
	}
	if t, perr := time.Parse(time.RFC3339Nano, at); perr == nil {
		r.GeneratedAt = t
	}
	r.Body = []byte(body)
	return r, nil
}
