package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBenchmarkRunNotFound is returned when no stored run has the given ID.
var ErrBenchmarkRunNotFound = errors.New("store: benchmark run not found")

const benchmarkRunColumns = `id, generated_at, version, commit_sha, dirty, host_summary, host_os, repeat, passed, failed, skipped, recorded_by, created_at`

// BenchmarkRunSummary is a stored run without its report body.
type BenchmarkRunSummary struct {
	ID          string    `json:"id"`
	GeneratedAt time.Time `json:"generated_at"`
	Version     string    `json:"version"`
	Commit      string    `json:"commit"`
	Dirty       bool      `json:"dirty"`
	HostSummary string    `json:"host_summary"`
	HostOS      string    `json:"host_os"`
	Repeat      int       `json:"repeat"`
	Passed      int       `json:"passed"`
	Failed      int       `json:"failed"`
	Skipped     int       `json:"skipped"`
	RecordedBy  string    `json:"recorded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// BenchmarkRun is a stored run with the results file it came from.
type BenchmarkRun struct {
	BenchmarkRunSummary
	Report []byte `json:"-"`
}

// RecordBenchmarkRun stores a run under its content derived ID and prunes the oldest runs beyond keep. Storing the same file again returns the existing ID.
func (db *DB) RecordBenchmarkRun(ctx context.Context, run BenchmarkRun, keep int) (id string, created bool, err error) {
	res, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO benchmark_runs (`+benchmarkRunColumns+`, report) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.GeneratedAt.UTC().Format(time.RFC3339Nano), run.Version, run.Commit, boolInt(run.Dirty), run.HostSummary, run.HostOS,
		run.Repeat, run.Passed, run.Failed, run.Skipped, run.RecordedBy, time.Now().UTC().Format(time.RFC3339Nano), string(run.Report))
	if err != nil {
		return "", false, fmt.Errorf("store: record benchmark run: %w", err)
	}
	n, _ := res.RowsAffected()
	if keep > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM benchmark_runs WHERE id IN (SELECT id FROM benchmark_runs ORDER BY generated_at DESC, id LIMIT -1 OFFSET ?)`, keep); err != nil {
			return run.ID, n > 0, fmt.Errorf("store: prune benchmark runs: %w", err)
		}
	}
	return run.ID, n > 0, nil
}

// ListBenchmarkRuns returns the newest runs first, at most limit rows.
func (db *DB) ListBenchmarkRuns(ctx context.Context, limit int) ([]BenchmarkRunSummary, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+benchmarkRunColumns+` FROM benchmark_runs ORDER BY generated_at DESC, id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list benchmark runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []BenchmarkRunSummary{}
	for rows.Next() {
		s, err := scanBenchmarkSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetBenchmarkRun returns one run with its results file.
func (db *DB) GetBenchmarkRun(ctx context.Context, id string) (BenchmarkRun, error) {
	row := db.QueryRowContext(ctx, `SELECT `+benchmarkRunColumns+`, report FROM benchmark_runs WHERE id = ?`, id)
	var run BenchmarkRun
	var report string
	s, err := scanBenchmarkSummary(row, &report)
	if errors.Is(err, sql.ErrNoRows) {
		return BenchmarkRun{}, ErrBenchmarkRunNotFound
	}
	if err != nil {
		return BenchmarkRun{}, err
	}
	run.BenchmarkRunSummary, run.Report = s, []byte(report)
	return run, nil
}

// DeleteBenchmarkRun removes one run.
func (db *DB) DeleteBenchmarkRun(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM benchmark_runs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete benchmark run: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBenchmarkRunNotFound
	}
	return nil
}

func scanBenchmarkSummary(row rowScanner, extra ...any) (BenchmarkRunSummary, error) {
	var s BenchmarkRunSummary
	var generated, created string
	var dirty int
	dest := append([]any{&s.ID, &generated, &s.Version, &s.Commit, &dirty, &s.HostSummary, &s.HostOS, &s.Repeat, &s.Passed, &s.Failed, &s.Skipped, &s.RecordedBy, &created}, extra...)
	if err := row.Scan(dest...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s, err
		}
		return s, fmt.Errorf("store: scan benchmark run: %w", err)
	}
	s.Dirty = dirty != 0
	s.GeneratedAt, _ = time.Parse(time.RFC3339Nano, generated)
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return s, nil
}
