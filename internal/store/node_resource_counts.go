package store

import (
	"context"
	"fmt"
)

// NodeResourceCounts returns apps plus databases per node_id. An empty key
// is the local node, which the caller folds in itself.
func (db *DB) NodeResourceCounts(ctx context.Context) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT node_id, COUNT(*) FROM (
			SELECT node_id FROM desired_services WHERE name NOT LIKE ? ESCAPE '\'
			UNION ALL
			SELECT node_id FROM desired_databases
		) GROUP BY node_id`, "%"+likeEscaper.Replace(CanaryServiceSuffix))
	if err != nil {
		return nil, fmt.Errorf("store: count resources per node: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: scan node resource count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}
