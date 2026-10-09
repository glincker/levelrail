package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Statuses of a DatabaseDataImport.
const (
	DataImportCopying  = "copying"
	DataImportVerified = "verified"
	DataImportFailed   = "failed"
)

// ErrDataImportInProgress is returned when a copy is already running.
var ErrDataImportInProgress = errors.New("a data copy is already running for this database")

// DatabaseDataImport is the latest copy of live data into one managed database
// (migrations/0381_database_data_imports.sql). Detail is the verification JSON.
type DatabaseDataImport struct {
	DatabaseName string
	Status       string
	Reason       string
	SourceHost   string
	SourcePort   int
	SourceDB     string
	Checked      int
	Mismatched   int
	Detail       string
	StartedAt    time.Time
	FinishedAt   time.Time
}

// ClaimDatabaseDataImport marks name as copying. It fails with
// ErrDataImportInProgress unless the previous run is finished or older than
// staleAfter, which covers a control plane that died mid-copy.
func (db *DB) ClaimDatabaseDataImport(ctx context.Context, name, host string, port int, srcDB string, now time.Time, staleAfter time.Duration) error {
	ts := now.UTC().Format(time.RFC3339)
	cutoff := now.Add(-staleAfter).UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `
		INSERT INTO database_data_imports (database_name, status, source_host, source_port, source_db, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(database_name) DO UPDATE SET status = excluded.status, reason = '', source_host = excluded.source_host,
			source_port = excluded.source_port, source_db = excluded.source_db, checked = 0, mismatched = 0, detail = '',
			started_at = excluded.started_at, finished_at = '', updated_at = excluded.updated_at
		WHERE database_data_imports.status != ? OR database_data_imports.started_at < ?
	`, name, DataImportCopying, host, port, srcDB, ts, ts, DataImportCopying, cutoff)
	if err != nil {
		return fmt.Errorf("store: claim data import for %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDataImportInProgress
	}
	return nil
}

// FinishDatabaseDataImport records the outcome of a claimed copy.
func (db *DB) FinishDatabaseDataImport(ctx context.Context, name, status, reason string, checked, mismatched int, detail string, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		UPDATE database_data_imports SET status = ?, reason = ?, checked = ?, mismatched = ?, detail = ?, finished_at = ?, updated_at = ?
		WHERE database_name = ?
	`, status, reason, checked, mismatched, detail, ts, ts, name)
	if err != nil {
		return fmt.Errorf("store: finish data import for %q: %w", name, err)
	}
	return nil
}

// GetDatabaseDataImport returns name's row and whether one exists.
func (db *DB) GetDatabaseDataImport(ctx context.Context, name string) (DatabaseDataImport, bool, error) {
	row := db.QueryRowContext(ctx, dataImportSelect+` WHERE database_name = ?`, name)
	d, err := scanDataImport(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseDataImport{}, false, nil
	}
	if err != nil {
		return DatabaseDataImport{}, false, fmt.Errorf("store: get data import for %q: %w", name, err)
	}
	return d, true, nil
}

// ListDatabaseDataImports returns every copy row.
func (db *DB) ListDatabaseDataImports(ctx context.Context) ([]DatabaseDataImport, error) {
	rows, err := db.QueryContext(ctx, dataImportSelect+` ORDER BY database_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list data imports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DatabaseDataImport
	for rows.Next() {
		d, err := scanDataImport(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list data imports: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const dataImportSelect = `SELECT database_name, status, reason, source_host, source_port, source_db, checked, mismatched, detail, started_at, finished_at FROM database_data_imports`

func scanDataImport(scan func(...any) error) (DatabaseDataImport, error) {
	var d DatabaseDataImport
	var started, finished string
	if err := scan(&d.DatabaseName, &d.Status, &d.Reason, &d.SourceHost, &d.SourcePort, &d.SourceDB, &d.Checked, &d.Mismatched, &d.Detail, &started, &finished); err != nil {
		return d, err
	}
	d.StartedAt, _ = time.Parse(time.RFC3339, started)
	if finished != "" {
		d.FinishedAt, _ = time.Parse(time.RFC3339, finished)
	}
	return d, nil
}
