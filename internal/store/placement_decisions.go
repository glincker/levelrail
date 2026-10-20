package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PlacementDecision is the latest recorded placement choice for one app or
// database. Body is the placement package's JSON, opaque to the store.
type PlacementDecision struct {
	Kind      string
	Resource  string
	Trigger   string
	NodeID    string
	DecidedAt time.Time
	Body      []byte
}

// SavePlacementDecision keeps only the newest decision per resource.
func (db *DB) SavePlacementDecision(ctx context.Context, d PlacementDecision) error {
	_, err := db.ExecContext(ctx, `INSERT INTO placement_decisions (kind, resource, trigger, node_id, decided_at, body)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (kind, resource) DO UPDATE SET trigger = excluded.trigger, node_id = excluded.node_id, decided_at = excluded.decided_at, body = excluded.body`,
		d.Kind, d.Resource, d.Trigger, d.NodeID, d.DecidedAt.UTC().Format(time.RFC3339Nano), string(d.Body))
	if err != nil {
		return fmt.Errorf("store: save placement decision %s/%s: %w", d.Kind, d.Resource, err)
	}
	return nil
}

// GetPlacementDecision returns the latest decision, or ok=false when none was recorded.
func (db *DB) GetPlacementDecision(ctx context.Context, kind, resource string) (PlacementDecision, bool, error) {
	var d PlacementDecision
	var at, body string
	err := db.QueryRowContext(ctx, `SELECT kind, resource, trigger, node_id, decided_at, body FROM placement_decisions WHERE kind = ? AND resource = ?`,
		kind, resource).Scan(&d.Kind, &d.Resource, &d.Trigger, &d.NodeID, &at, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return PlacementDecision{}, false, nil
	}
	if err != nil {
		return PlacementDecision{}, false, fmt.Errorf("store: get placement decision %s/%s: %w", kind, resource, err)
	}
	t, perr := time.Parse(time.RFC3339Nano, at)
	if perr != nil {
		return PlacementDecision{}, false, fmt.Errorf("store: parse placement decision time %q: %w", at, perr)
	}
	d.DecidedAt, d.Body = t, []byte(body)
	return d, true, nil
}
