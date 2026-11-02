package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// RollupCount is one ranked label inside a stored rollup.
type RollupCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// VisitorRollup is one hour of traffic for an app and domain. Visitors counts
// addresses first seen in their salt period during this hour, so summing hours
// gives unique visitors per period.
type VisitorRollup struct {
	App       string
	Domain    string
	HourStart time.Time
	Epoch     int64
	Requests  int
	Status    [4]int
	Visitors  int
	TopPaths  []RollupCount
	TopRefs   []RollupCount
}

// UpsertVisitorRollups writes rows in one transaction, replacing a row with the same key.
func (db *DB) UpsertVisitorRollups(ctx context.Context, rows []VisitorRollup) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: upsert visitor rollups: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range rows {
		paths, _ := json.Marshal(nonNilCounts(r.TopPaths))
		refs, _ := json.Marshal(nonNilCounts(r.TopRefs))
		if _, err := tx.ExecContext(ctx, `INSERT INTO visitor_rollups
			(app, domain, hour_start, epoch, requests, s2xx, s3xx, s4xx, s5xx, visitors, top_paths, top_refs)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (app, domain, hour_start, epoch) DO UPDATE SET
			requests = excluded.requests, s2xx = excluded.s2xx, s3xx = excluded.s3xx, s4xx = excluded.s4xx,
			s5xx = excluded.s5xx, visitors = excluded.visitors, top_paths = excluded.top_paths, top_refs = excluded.top_refs`,
			r.App, r.Domain, r.HourStart.Unix(), r.Epoch, r.Requests, r.Status[0], r.Status[1], r.Status[2], r.Status[3],
			r.Visitors, string(paths), string(refs)); err != nil {
			return fmt.Errorf("store: upsert visitor rollup for %q: %w", r.App, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: upsert visitor rollups: commit: %w", err)
	}
	return nil
}

// ListVisitorRollups returns rows for app with hour_start in [since, until), oldest first.
// An empty domain returns every domain.
func (db *DB) ListVisitorRollups(ctx context.Context, app, domain string, since, until time.Time) ([]VisitorRollup, error) {
	q := `SELECT app, domain, hour_start, epoch, requests, s2xx, s3xx, s4xx, s5xx, visitors, top_paths, top_refs
		FROM visitor_rollups WHERE app = ? AND hour_start >= ? AND hour_start < ?`
	args := []any{app, since.Unix(), until.Unix()}
	if domain != "" {
		q += ` AND domain = ?`
		args = append(args, domain)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY hour_start`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list visitor rollups for %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []VisitorRollup
	for rows.Next() {
		var r VisitorRollup
		var hour int64
		var paths, refs string
		if err := rows.Scan(&r.App, &r.Domain, &hour, &r.Epoch, &r.Requests, &r.Status[0], &r.Status[1], &r.Status[2], &r.Status[3], &r.Visitors, &paths, &refs); err != nil {
			return nil, fmt.Errorf("store: scan visitor rollup: %w", err)
		}
		r.HourStart = time.Unix(hour, 0).UTC()
		_ = json.Unmarshal([]byte(paths), &r.TopPaths)
		_ = json.Unmarshal([]byte(refs), &r.TopRefs)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list visitor rollups for %q: %w", app, err)
	}
	return out, nil
}

// PruneVisitorRollups deletes rows whose hour started before cutoff and returns how many went.
func (db *DB) PruneVisitorRollups(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM visitor_rollups WHERE hour_start < ?`, cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("store: prune visitor rollups: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func nonNilCounts(c []RollupCount) []RollupCount {
	if c == nil {
		return []RollupCount{}
	}
	return c
}
