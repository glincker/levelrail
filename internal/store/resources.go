package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Change operations of the resource change log; kinds reuse ResourceKind*.
const (
	ResourceOpAdded    = "ADDED"
	ResourceOpModified = "MODIFIED"
	ResourceOpDeleted  = "DELETED"
)

// Env names and defaults for change log retention.
const (
	EnvResourceChangesRetain       = "APP_RESOURCE_CHANGES_RETAIN"
	EnvResourceChangesMaxAge       = "APP_RESOURCE_CHANGES_MAX_AGE"
	EnvResourceChangesCompactEvery = "APP_RESOURCE_CHANGES_COMPACT_EVERY"

	defaultResourceChangesRetain       = 10000
	defaultResourceChangesMaxAge       = 7 * 24 * time.Hour
	defaultResourceChangesCompactEvery = 100
)

const resourceTimeLayout = "2006-01-02T15:04:05.000Z"

var (
	// ErrResourceVersionCompacted means the requested version is older than retained history.
	ErrResourceVersionCompacted = errors.New("store: resource version compacted")
	// ErrResourceVersionConflict means a compare-and-swap saw a newer version.
	ErrResourceVersionConflict = errors.New("store: resource version conflict")
)

// ResourceChange is one row of the resource change log.
type ResourceChange struct {
	Version   int64
	Kind      string
	Namespace string
	Name      string
	Op        string
	Object    json.RawMessage
	At        time.Time
}

// ResourceRetention is the change log compaction policy.
type ResourceRetention struct {
	Retain       int
	MaxAge       time.Duration
	CompactEvery int
}

// ResourceRetentionFromEnv reads the compaction policy, falling back to defaults.
func ResourceRetentionFromEnv() ResourceRetention {
	r := ResourceRetention{
		Retain:       defaultResourceChangesRetain,
		MaxAge:       defaultResourceChangesMaxAge,
		CompactEvery: defaultResourceChangesCompactEvery,
	}
	if n, err := strconv.Atoi(os.Getenv(EnvResourceChangesRetain)); err == nil && n > 0 {
		r.Retain = n
	}
	if d, err := time.ParseDuration(os.Getenv(EnvResourceChangesMaxAge)); err == nil && d > 0 {
		r.MaxAge = d
	}
	if n, err := strconv.Atoi(os.Getenv(EnvResourceChangesCompactEvery)); err == nil && n > 0 {
		r.CompactEvery = n
	}
	return r
}

// resourceQuerier is satisfied by both *DB and *sql.Tx, so a change can join the caller's transaction.
type resourceQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func appendResourceChange(ctx context.Context, q resourceQuerier, c ResourceChange) (int64, error) {
	obj := c.Object
	if len(obj) == 0 {
		obj = json.RawMessage("{}")
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO resource_changes (kind, namespace, name, op, object) VALUES (?, ?, ?, ?, ?)
	`, c.Kind, c.Namespace, c.Name, c.Op, string(obj))
	if err != nil {
		return 0, fmt.Errorf("store: append %s/%s change: %w", c.Kind, c.Name, err)
	}
	v, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: append %s/%s change: version: %w", c.Kind, c.Name, err)
	}
	return v, nil
}

func latestObjectVersion(ctx context.Context, q resourceQuerier, kind, name string) (int64, string, error) {
	var (
		v  int64
		op string
	)
	err := q.QueryRowContext(ctx, `
		SELECT version, op FROM resource_changes WHERE kind = ? AND name = ? ORDER BY version DESC LIMIT 1
	`, kind, name).Scan(&v, &op)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("store: latest version of %s/%s: %w", kind, name, err)
	}
	return v, op, nil
}

// RecordResourceChange appends c and returns its version.
func (db *DB) RecordResourceChange(ctx context.Context, c ResourceChange) (int64, error) {
	return appendResourceChange(ctx, db, c)
}

// RecordResourceChangeIf appends c only while the object's latest version is
// still expected (0 for "never recorded"), the compare-and-swap for v2 writes.
func (db *DB) RecordResourceChangeIf(ctx context.Context, c ResourceChange, expected int64) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: record %s/%s change: begin: %w", c.Kind, c.Name, err)
	}
	defer func() { _ = tx.Rollback() }()
	cur, _, err := latestObjectVersion(ctx, tx, c.Kind, c.Name)
	if err != nil {
		return 0, err
	}
	if cur != expected {
		return 0, fmt.Errorf("%w: %s/%s is at %d, not %d", ErrResourceVersionConflict, c.Kind, c.Name, cur, expected)
	}
	v, err := appendResourceChange(ctx, tx, c)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: record %s/%s change: commit: %w", c.Kind, c.Name, err)
	}
	return v, nil
}

// CurrentResourceVersion is the last version ever allocated, 0 before any change.
func (db *DB) CurrentResourceVersion(ctx context.Context) (int64, error) {
	var v int64
	err := db.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = 'resource_changes'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: current resource version: %w", err)
	}
	return v, nil
}

// ResourceCompactedThrough is the watermark at or below which history may be missing.
func (db *DB) ResourceCompactedThrough(ctx context.Context) (int64, error) {
	return compactedThrough(ctx, db)
}

func compactedThrough(ctx context.Context, q resourceQuerier) (int64, error) {
	var v int64
	if err := q.QueryRowContext(ctx, `SELECT compacted_through FROM resource_compaction WHERE id = 1`).Scan(&v); err != nil {
		return 0, fmt.Errorf("store: resource compaction watermark: %w", err)
	}
	return v, nil
}

// ResourceChangesSince returns up to limit changes of kind after version, oldest first.
// It returns ErrResourceVersionCompacted when after predates retained history.
func (db *DB) ResourceChangesSince(ctx context.Context, kind string, after int64, limit int) ([]ResourceChange, error) {
	mark, err := compactedThrough(ctx, db)
	if err != nil {
		return nil, err
	}
	if after < mark {
		return nil, fmt.Errorf("%w: %d is older than %d", ErrResourceVersionCompacted, after, mark)
	}
	if limit <= 0 {
		limit = -1
	}
	rows, err := db.QueryContext(ctx, `
		SELECT version, kind, namespace, name, op, object, at FROM resource_changes
		WHERE kind = ? AND version > ? ORDER BY version LIMIT ?
	`, kind, after, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list %s changes: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	var out []ResourceChange
	for rows.Next() {
		var (
			c       ResourceChange
			obj, at string
		)
		if err := rows.Scan(&c.Version, &c.Kind, &c.Namespace, &c.Name, &c.Op, &obj, &at); err != nil {
			return nil, fmt.Errorf("store: scan %s change: %w", kind, err)
		}
		c.Object = json.RawMessage(obj)
		c.At, _ = time.Parse(resourceTimeLayout, at)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list %s changes: %w", kind, err)
	}
	return out, nil
}

// LatestResourceVersions maps each name of kind to its newest change version.
func (db *DB) LatestResourceVersions(ctx context.Context, kind string) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, MAX(version) FROM resource_changes WHERE kind = ? GROUP BY name`, kind)
	if err != nil {
		return nil, fmt.Errorf("store: latest %s versions: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var (
			name string
			v    int64
		)
		if err := rows.Scan(&name, &v); err != nil {
			return nil, fmt.Errorf("store: scan %s version: %w", kind, err)
		}
		out[name] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: latest %s versions: %w", kind, err)
	}
	return out, nil
}

// CompactResourceChanges trims history beyond the retained count or age and
// returns the rows removed. The newest row of each live object is kept so its
// metadata.resourceVersion survives.
func (db *DB) CompactResourceChanges(ctx context.Context, r ResourceRetention, now time.Time) (int64, error) {
	return compactResourceChanges(ctx, db, r, now)
}

func compactResourceChanges(ctx context.Context, q resourceQuerier, r ResourceRetention, now time.Time) (int64, error) {
	var cutoff int64
	err := q.QueryRowContext(ctx, `
		SELECT MAX(
			COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'resource_changes'), 0) - ?,
			COALESCE((SELECT MAX(version) FROM resource_changes WHERE at < ?), 0)
		)
	`, r.Retain, now.Add(-r.MaxAge).UTC().Format(resourceTimeLayout)).Scan(&cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: compact resource changes: cutoff: %w", err)
	}
	mark, err := compactedThrough(ctx, q)
	if err != nil {
		return 0, err
	}
	if cutoff <= mark {
		return 0, nil
	}
	res, err := q.ExecContext(ctx, `
		DELETE FROM resource_changes WHERE version <= ? AND version NOT IN (
			SELECT MAX(version) FROM resource_changes GROUP BY kind, name
		) OR (version <= ? AND op = 'DELETED')
	`, cutoff, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: compact resource changes: %w", err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE resource_compaction SET compacted_through = ? WHERE id = 1`, cutoff); err != nil {
		return 0, fmt.Errorf("store: compact resource changes: watermark: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
