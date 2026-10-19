package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// IdempotencyRecord is one cached response for a retried mutating call.
type IdempotencyRecord struct {
	Principal   string
	Key         string
	Route       string
	BodyHash    string
	Status      int
	ContentType string
	Response    []byte
	CreatedAt   time.Time
}

// GetIdempotencyRecord returns the cached record, or found=false when none exists.
func (db *DB) GetIdempotencyRecord(ctx context.Context, principal, key string) (rec IdempotencyRecord, found bool, err error) {
	var created string
	err = db.QueryRowContext(ctx,
		`SELECT principal, key, route, body_hash, status, content_type, response, created_at FROM idempotency_keys WHERE principal = ? AND key = ?`,
		principal, key).Scan(&rec.Principal, &rec.Key, &rec.Route, &rec.BodyHash, &rec.Status, &rec.ContentType, &rec.Response, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("store: get idempotency record: %w", err)
	}
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return rec, true, nil
}

// PutIdempotencyRecord stores a record; the first writer for a key wins.
func (db *DB) PutIdempotencyRecord(ctx context.Context, rec IdempotencyRecord) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO idempotency_keys (principal, key, route, body_hash, status, content_type, response, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Principal, rec.Key, rec.Route, rec.BodyHash, rec.Status, rec.ContentType, rec.Response, rec.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: put idempotency record: %w", err)
	}
	return nil
}

// PruneIdempotencyRecords deletes records created before cutoff.
func (db *DB) PruneIdempotencyRecords(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: prune idempotency records: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
