package store

import (
	"context"
	"fmt"
)

// NodesWithRunningVolumeMoves returns node ids that are the source or target of
// a volume move still in progress, so they may hold the only copy of a volume.
func (db *DB) NodesWithRunningVolumeMoves(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	for _, q := range []string{
		`SELECT from_node_id, to_node_id FROM app_volume_moves WHERE status = 'running'`,
		`SELECT from_node_id, to_node_id FROM node_moves WHERE status = 'running' AND stateful = 1`,
	} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("store: running volume moves: %w", err)
		}
		for rows.Next() {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("store: scan running volume move: %w", err)
			}
			out[a], out[b] = true, true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return out, nil
}
