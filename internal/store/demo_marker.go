package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDemoMarkerNotFound is returned by GetDemoMarker on an instance the demo
// seeder never touched.
var ErrDemoMarkerNotFound = errors.New("store: demo seed marker not found")

// DemoMarker records that this data dir was produced by the demo seeder.
type DemoMarker struct {
	Seed     int64
	Anchor   time.Time
	SeededAt time.Time
}

// SaveDemoMarker writes the single marker row, replacing any previous one.
func (db *DB) SaveDemoMarker(ctx context.Context, m DemoMarker) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO demo_seed (id, seed, anchor, seeded_at) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET seed = excluded.seed, anchor = excluded.anchor, seeded_at = excluded.seeded_at
	`, m.Seed, m.Anchor.UTC().Format(time.RFC3339Nano), m.SeededAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save demo marker: %w", err)
	}
	return nil
}

// GetDemoMarker returns the marker row, or ErrDemoMarkerNotFound.
func (db *DB) GetDemoMarker(ctx context.Context) (DemoMarker, error) {
	var m DemoMarker
	var anchor, seededAt string
	err := db.QueryRowContext(ctx, `SELECT seed, anchor, seeded_at FROM demo_seed WHERE id = 1`).Scan(&m.Seed, &anchor, &seededAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DemoMarker{}, ErrDemoMarkerNotFound
	}
	if err != nil {
		return DemoMarker{}, fmt.Errorf("store: get demo marker: %w", err)
	}
	if m.Anchor, err = time.Parse(time.RFC3339Nano, anchor); err != nil {
		return DemoMarker{}, fmt.Errorf("store: parse demo marker anchor: %w", err)
	}
	if m.SeededAt, err = time.Parse(time.RFC3339Nano, seededAt); err != nil {
		return DemoMarker{}, fmt.Errorf("store: parse demo marker seeded_at: %w", err)
	}
	return m, nil
}
