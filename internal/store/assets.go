package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrAssetNotFound is returned when a bucket has no index row for a key.
var ErrAssetNotFound = errors.New("store: asset not found")

// Asset is one index row for an object brokered through levelrail.
type Asset struct {
	BucketID    string
	Key         string
	SizeBytes   int64
	ContentType string
	ETag        string
	Uploader    string
	CreatedAt   string
	LastSeenAt  string
}

const assetCols = `bucket_id, key, size_bytes, content_type, etag, uploader, created_at, last_seen_at`

func scanAsset(sc rowScanner) (Asset, error) {
	var a Asset
	err := sc.Scan(&a.BucketID, &a.Key, &a.SizeBytes, &a.ContentType, &a.ETag, &a.Uploader, &a.CreatedAt, &a.LastSeenAt)
	return a, err
}

// UpsertAsset records an object after a HEAD or listing confirmed it. CreatedAt and Uploader of an existing row are kept.
func (db *DB) UpsertAsset(ctx context.Context, a Asset) (Asset, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT INTO assets (`+assetCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (bucket_id, key) DO UPDATE SET
			size_bytes = excluded.size_bytes, content_type = excluded.content_type, etag = excluded.etag,
			uploader = CASE WHEN assets.uploader = '' THEN excluded.uploader ELSE assets.uploader END,
			last_seen_at = excluded.last_seen_at
	`, a.BucketID, a.Key, a.SizeBytes, a.ContentType, a.ETag, a.Uploader, now, now)
	if err != nil {
		return Asset{}, fmt.Errorf("store: upsert asset %q/%q: %w", a.BucketID, a.Key, err)
	}
	return db.GetAsset(ctx, a.BucketID, a.Key)
}

// GetAsset returns one index row, or ErrAssetNotFound.
func (db *DB) GetAsset(ctx context.Context, bucketID, key string) (Asset, error) {
	a, err := scanAsset(db.QueryRowContext(ctx, `SELECT `+assetCols+` FROM assets WHERE bucket_id = ? AND key = ?`, bucketID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrAssetNotFound
	}
	if err != nil {
		return Asset{}, fmt.Errorf("store: get asset %q/%q: %w", bucketID, key, err)
	}
	return a, nil
}

// ListAssets returns index rows by key after the given key, at most limit rows (limit <= 0 means all).
func (db *DB) ListAssets(ctx context.Context, bucketID, afterKey string, limit int) ([]Asset, error) {
	q := `SELECT ` + assetCols + ` FROM assets WHERE bucket_id = ? AND key > ? ORDER BY key`
	args := []any{bucketID, afterKey}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list assets %q: %w", bucketID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan asset: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAsset drops an index row; a missing row is not an error because listing prunes stale rows too.
func (db *DB) DeleteAsset(ctx context.Context, bucketID, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM assets WHERE bucket_id = ? AND key = ?`, bucketID, key); err != nil {
		return fmt.Errorf("store: delete asset %q/%q: %w", bucketID, key, err)
	}
	return nil
}

// AssetTotals sums the indexed rows of a bucket.
func (db *DB) AssetTotals(ctx context.Context, bucketID string) (objects, bytes int64, err error) {
	err = db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM assets WHERE bucket_id = ?`, bucketID).Scan(&objects, &bytes)
	if err != nil {
		return 0, 0, fmt.Errorf("store: asset totals %q: %w", bucketID, err)
	}
	return objects, bytes, nil
}
