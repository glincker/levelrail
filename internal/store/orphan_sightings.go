package store

import (
	"context"
	"fmt"
	"time"
)

// OrphanSighting records when a leftover resource was first seen.
type OrphanSighting struct {
	Kind        string
	NodeID      string
	Name        string
	FirstSeenAt time.Time
}

// RecordOrphanSighting stores the first sighting of a resource and keeps the
// original timestamp on every later call.
func (db *DB) RecordOrphanSighting(ctx context.Context, kind, nodeID, name string, at time.Time) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO orphan_sightings (kind, node_id, name, first_seen_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(kind, node_id, name) DO NOTHING`,
		kind, nodeID, name, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: record orphan sighting %s/%q: %w", kind, name, err)
	}
	return nil
}

// ClearOrphanSighting forgets a resource once it is gone or no longer orphaned.
func (db *DB) ClearOrphanSighting(ctx context.Context, kind, nodeID, name string) error {
	if _, err := db.ExecContext(ctx,
		`DELETE FROM orphan_sightings WHERE kind = ? AND node_id = ? AND name = ?`, kind, nodeID, name); err != nil {
		return fmt.Errorf("store: clear orphan sighting %s/%q: %w", kind, name, err)
	}
	return nil
}

// ListOrphanSightings returns every recorded sighting.
func (db *DB) ListOrphanSightings(ctx context.Context) ([]OrphanSighting, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, node_id, name, first_seen_at FROM orphan_sightings`)
	if err != nil {
		return nil, fmt.Errorf("store: list orphan sightings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []OrphanSighting
	for rows.Next() {
		var (
			s  OrphanSighting
			ts string
		)
		if err := rows.Scan(&s.Kind, &s.NodeID, &s.Name, &ts); err != nil {
			return nil, fmt.Errorf("store: scan orphan sighting: %w", err)
		}
		s.FirstSeenAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate orphan sightings: %w", err)
	}
	return out, nil
}
