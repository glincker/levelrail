package store

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvCachePurgeKeep bounds how many purge records are kept per app.
const EnvCachePurgeKeep = "APP_CACHE_PURGE_HISTORY_KEEP"

const (
	defaultCachePurgeKeep = 200
	cachePurgeListLimit   = 100
)

// CachePurge is one cache purge: who removed what from which domain
// (migrations/1050). Value is the URL, prefix or tag; empty for all.
type CachePurge struct {
	ID      int64
	App     string
	Domain  string
	Scope   string
	Value   string
	Removed int
	Actor   string
	At      time.Time
}

func cachePurgeKeep() int {
	if n, err := strconv.Atoi(os.Getenv(EnvCachePurgeKeep)); err == nil && n > 0 {
		return n
	}
	return defaultCachePurgeKeep
}

// AddCachePurge stores p and trims the app's history to the retention bound.
func (db *DB) AddCachePurge(ctx context.Context, p CachePurge) error {
	if p.At.IsZero() {
		p.At = time.Now()
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO cache_purges (app, domain, scope, value, removed, actor, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, p.App, p.Domain, p.Scope, p.Value, p.Removed, p.Actor, p.At.UTC().Format(appEventTimeFormat)); err != nil {
		return fmt.Errorf("store: add cache purge for %q: %w", p.App, err)
	}
	if _, err := db.ExecContext(ctx, `
		DELETE FROM cache_purges WHERE app = ? AND id NOT IN (
			SELECT id FROM cache_purges WHERE app = ? ORDER BY id DESC LIMIT ?)
	`, p.App, p.App, cachePurgeKeep()); err != nil {
		return fmt.Errorf("store: trim cache purges for %q: %w", p.App, err)
	}
	return nil
}

// ListCachePurges returns app's purges newest first; an empty app lists
// every app's.
func (db *DB) ListCachePurges(ctx context.Context, app string, limit int) ([]CachePurge, error) {
	if limit <= 0 || limit > cachePurgeListLimit {
		limit = cachePurgeListLimit
	}
	q := `SELECT id, app, domain, scope, value, removed, actor, created_at FROM cache_purges`
	var args []any
	if app != "" {
		q += ` WHERE app = ?`
		args = append(args, app)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list cache purges: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CachePurge
	for rows.Next() {
		var p CachePurge
		var at string
		if err := rows.Scan(&p.ID, &p.App, &p.Domain, &p.Scope, &p.Value, &p.Removed, &p.Actor, &at); err != nil {
			return nil, fmt.Errorf("store: scan cache purge: %w", err)
		}
		p.At, _ = time.Parse(appEventTimeFormat, at)
		out = append(out, p)
	}
	return out, rows.Err()
}
