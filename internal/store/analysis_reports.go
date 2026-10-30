package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrAnalysisReportNotFound means no report has the requested id.
var ErrAnalysisReportNotFound = errors.New("store: analysis report not found")

// AnalysisReport is one stored report; Report is the JSON the analysis
// package marshals, opaque to the store.
type AnalysisReport struct {
	ID            string
	App           string
	TriggerKind   string
	TriggerID     string
	Status        string
	SchemaVersion int
	Report        []byte
	CreatedAt     time.Time
}

const analysisReportColumns = `id, app, trigger_kind, trigger_id, status, schema_version, report, created_at`

// SaveAnalysisReport inserts r, stamping CreatedAt when unset.
func (db *DB) SaveAnalysisReport(ctx context.Context, r AnalysisReport) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `INSERT INTO analysis_reports (`+analysisReportColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.App, r.TriggerKind, r.TriggerID, r.Status, r.SchemaVersion, string(r.Report), r.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save analysis report %q: %w", r.ID, err)
	}
	return nil
}

// GetAnalysisReport returns the report with id, or ErrAnalysisReportNotFound.
func (db *DB) GetAnalysisReport(ctx context.Context, id string) (AnalysisReport, error) {
	row := db.QueryRowContext(ctx, `SELECT `+analysisReportColumns+` FROM analysis_reports WHERE id = ?`, id)
	r, err := scanAnalysisReport(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisReport{}, ErrAnalysisReportNotFound
	}
	if err != nil {
		return AnalysisReport{}, fmt.Errorf("store: get analysis report %q: %w", id, err)
	}
	return r, nil
}

// ListAnalysisReports returns an app's reports newest first, capped at limit.
func (db *DB) ListAnalysisReports(ctx context.Context, app string, limit int) ([]AnalysisReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT `+analysisReportColumns+` FROM analysis_reports WHERE app = ? ORDER BY created_at DESC, id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list analysis reports for %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []AnalysisReport
	for rows.Next() {
		r, err := scanAnalysisReport(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan analysis report: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneAnalysisReports deletes reports older than cutoff and reports how many went.
func (db *DB) PruneAnalysisReports(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM analysis_reports WHERE created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: prune analysis reports: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: prune analysis reports: %w", err)
	}
	return n, nil
}

func scanAnalysisReport(scan func(dest ...any) error) (AnalysisReport, error) {
	var (
		r       AnalysisReport
		body    string
		created string
	)
	if err := scan(&r.ID, &r.App, &r.TriggerKind, &r.TriggerID, &r.Status, &r.SchemaVersion, &body, &created); err != nil {
		return AnalysisReport{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return AnalysisReport{}, fmt.Errorf("parse created_at %q: %w", created, err)
	}
	r.CreatedAt = t
	r.Report = []byte(body)
	return r, nil
}
