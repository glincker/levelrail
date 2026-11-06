// Package diskhistory keeps a small time series of data dir free space and
// per-category Docker bytes, and forecasts when the disk fills.
package diskhistory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Sample is one recorded moment. Category fields are 0 when Docker usage was unavailable.
type Sample struct {
	At              time.Time `json:"at"`
	FreeBytes       int64     `json:"free_bytes"`
	TotalBytes      int64     `json:"total_bytes"`
	ImagesBytes     int64     `json:"images_bytes"`
	ContainersBytes int64     `json:"containers_bytes"`
	VolumesBytes    int64     `json:"volumes_bytes"`
	BuildCacheBytes int64     `json:"build_cache_bytes"`
}

// Store persists samples in the control plane database.
type Store struct{ db *sql.DB }

// NewStore wraps the shared database handle.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Insert records one sample; a repeat of the same instant replaces the earlier row.
func (s *Store) Insert(ctx context.Context, sm Sample) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO disk_history_samples
		(sampled_at, free_bytes, total_bytes, images_bytes, containers_bytes, volumes_bytes, build_cache_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sm.At.UTC().Format(time.RFC3339Nano), sm.FreeBytes, sm.TotalBytes,
		sm.ImagesBytes, sm.ContainersBytes, sm.VolumesBytes, sm.BuildCacheBytes)
	if err != nil {
		return fmt.Errorf("diskhistory: insert sample: %w", err)
	}
	return nil
}

// Since returns samples at or after since, oldest first.
func (s *Store) Since(ctx context.Context, since time.Time) ([]Sample, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sampled_at, free_bytes, total_bytes, images_bytes, containers_bytes, volumes_bytes, build_cache_bytes
		FROM disk_history_samples WHERE sampled_at >= ? ORDER BY sampled_at ASC`,
		since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("diskhistory: list samples: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Sample
	for rows.Next() {
		var sm Sample
		var at string
		if err := rows.Scan(&at, &sm.FreeBytes, &sm.TotalBytes, &sm.ImagesBytes, &sm.ContainersBytes, &sm.VolumesBytes, &sm.BuildCacheBytes); err != nil {
			return nil, fmt.Errorf("diskhistory: scan sample: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			continue
		}
		sm.At = t
		out = append(out, sm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("diskhistory: list samples: %w", err)
	}
	return out, nil
}

// Latest returns the newest sample time, or the zero time when none exist.
func (s *Store) Latest(ctx context.Context) (time.Time, error) {
	var at sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(sampled_at) FROM disk_history_samples`).Scan(&at); err != nil {
		return time.Time{}, fmt.Errorf("diskhistory: latest sample: %w", err)
	}
	if !at.Valid {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

// Prune deletes samples older than cutoff and returns how many went.
func (s *Store) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM disk_history_samples WHERE sampled_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("diskhistory: prune samples: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
