package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrPipelineArtifactNotFound is returned when an artifact ID is unknown.
var ErrPipelineArtifactNotFound = errors.New("store: pipeline artifact not found")

// PipelineArtifact is the index row for one archive a job uploaded.
type PipelineArtifact struct {
	ID          string
	RunID       string
	JobKey      string
	Name        string
	SizeBytes   int64
	SHA256      string
	StoragePath string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const pipelineArtifactCols = `id, run_id, job_key, name, size_bytes, sha256, storage_path, created_at, expires_at`

func scanPipelineArtifact(scan func(...any) error) (PipelineArtifact, error) {
	var a PipelineArtifact
	var created, expires string
	if err := scan(&a.ID, &a.RunID, &a.JobKey, &a.Name, &a.SizeBytes, &a.SHA256, &a.StoragePath, &created, &expires); err != nil {
		return PipelineArtifact{}, err
	}
	var err error
	if a.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return PipelineArtifact{}, fmt.Errorf("parse created_at: %w", err)
	}
	if a.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
		return PipelineArtifact{}, fmt.Errorf("parse expires_at: %w", err)
	}
	return a, nil
}

// SavePipelineArtifact records a job's artifact, replacing the earlier one
// when the job is retried.
func (db *DB) SavePipelineArtifact(ctx context.Context, a PipelineArtifact) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO pipeline_artifacts (`+pipelineArtifactCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, job_key) DO UPDATE SET id = excluded.id, name = excluded.name, size_bytes = excluded.size_bytes,
			sha256 = excluded.sha256, storage_path = excluded.storage_path, created_at = excluded.created_at, expires_at = excluded.expires_at
	`, a.ID, a.RunID, a.JobKey, a.Name, a.SizeBytes, a.SHA256, a.StoragePath, formatTime(a.CreatedAt), formatTime(a.ExpiresAt)); err != nil {
		return fmt.Errorf("store: save pipeline artifact: %w", err)
	}
	return nil
}

// ListPipelineArtifacts returns a run's artifacts in job order.
func (db *DB) ListPipelineArtifacts(ctx context.Context, runID string) ([]PipelineArtifact, error) {
	return db.queryPipelineArtifacts(ctx, `WHERE run_id = ? ORDER BY job_key`, runID)
}

// ListExpiredPipelineArtifacts returns artifacts whose retention ended.
func (db *DB) ListExpiredPipelineArtifacts(ctx context.Context, now time.Time) ([]PipelineArtifact, error) {
	return db.queryPipelineArtifacts(ctx, `WHERE expires_at <= ? ORDER BY expires_at`, formatTime(now))
}

// GetPipelineArtifact returns one artifact, or ErrPipelineArtifactNotFound.
func (db *DB) GetPipelineArtifact(ctx context.Context, id string) (PipelineArtifact, error) {
	a, err := scanPipelineArtifact(db.QueryRowContext(ctx, `SELECT `+pipelineArtifactCols+` FROM pipeline_artifacts WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PipelineArtifact{}, ErrPipelineArtifactNotFound
	}
	if err != nil {
		return PipelineArtifact{}, fmt.Errorf("store: get pipeline artifact %q: %w", id, err)
	}
	return a, nil
}

// DeletePipelineArtifact removes the index row.
func (db *DB) DeletePipelineArtifact(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM pipeline_artifacts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete pipeline artifact %q: %w", id, err)
	}
	return nil
}

func (db *DB) queryPipelineArtifacts(ctx context.Context, where string, args ...any) ([]PipelineArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+pipelineArtifactCols+` FROM pipeline_artifacts `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query pipeline artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineArtifact
	for rows.Next() {
		a, err := scanPipelineArtifact(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan pipeline artifact: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
