package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDeployTimelineNotFound means no timeline was frozen for the attempt.
var ErrDeployTimelineNotFound = errors.New("store: deploy timeline not found")

// DeployTimelineRow is one frozen timeline: Payload is the JSON the
// deploytimeline package marshals, opaque to the store.
type DeployTimelineRow struct {
	AttemptID   string
	ServiceName string
	FrozenAt    time.Time
	Payload     []byte
}

const deployTimelineColumns = `attempt_id, service_name, frozen_at, payload`

// SaveDeployTimeline stores row, keeping the first freeze when one exists so a
// replay never changes after the fact.
func (db *DB) SaveDeployTimeline(ctx context.Context, row DeployTimelineRow) error {
	if row.FrozenAt.IsZero() {
		row.FrozenAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_timelines (`+deployTimelineColumns+`) VALUES (?, ?, ?, ?)
		ON CONFLICT (attempt_id) DO NOTHING
	`, row.AttemptID, row.ServiceName, row.FrozenAt.UTC().Format(time.RFC3339Nano), string(row.Payload))
	if err != nil {
		return fmt.Errorf("store: save deploy timeline %q: %w", row.AttemptID, err)
	}
	return nil
}

// GetDeployTimeline returns the frozen timeline for attemptID, or
// ErrDeployTimelineNotFound.
func (db *DB) GetDeployTimeline(ctx context.Context, attemptID string) (DeployTimelineRow, error) {
	var (
		row    DeployTimelineRow
		frozen string
		body   string
	)
	err := db.QueryRowContext(ctx, `SELECT `+deployTimelineColumns+` FROM deploy_timelines WHERE attempt_id = ?`, attemptID).
		Scan(&row.AttemptID, &row.ServiceName, &frozen, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return DeployTimelineRow{}, ErrDeployTimelineNotFound
	}
	if err != nil {
		return DeployTimelineRow{}, fmt.Errorf("store: get deploy timeline %q: %w", attemptID, err)
	}
	if row.FrozenAt, err = time.Parse(time.RFC3339Nano, frozen); err != nil {
		return DeployTimelineRow{}, fmt.Errorf("store: parse deploy timeline frozen_at %q: %w", frozen, err)
	}
	row.Payload = []byte(body)
	return row, nil
}

// PruneDeployTimelines deletes timelines frozen before cutoff and reports how
// many went.
func (db *DB) PruneDeployTimelines(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM deploy_timelines WHERE frozen_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: prune deploy timelines: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: prune deploy timelines: %w", err)
	}
	return n, nil
}
