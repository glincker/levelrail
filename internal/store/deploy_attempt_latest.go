package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LatestDeployActivity returns, per service, the finish time of its newest
// deploy attempt (its start time while unfinished) in one statement, the
// batch counterpart of ListDeployAttempts()[0] for list views.
func (db *DB) LatestDeployActivity(ctx context.Context) (map[string]time.Time, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.service_name, a.started_at, a.finished_at
		FROM desired_services s
		JOIN deploy_attempts a ON a.id = (
			SELECT id FROM deploy_attempts WHERE service_name = s.name ORDER BY started_at DESC LIMIT 1
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("store: latest deploy activity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]time.Time)
	for rows.Next() {
		var name, started string
		var finished sql.NullString
		if err := rows.Scan(&name, &started, &finished); err != nil {
			return nil, fmt.Errorf("store: scan latest deploy activity: %w", err)
		}
		raw := started
		if finished.Valid {
			raw = finished.String
		}
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, fmt.Errorf("store: parse latest deploy activity %q: %w", raw, err)
		}
		out[name] = t
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate latest deploy activity: %w", err)
	}
	return out, nil
}
