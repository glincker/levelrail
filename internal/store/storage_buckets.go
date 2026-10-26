package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrStorageBucketNotFound is returned when a bucket id has no row.
	ErrStorageBucketNotFound = errors.New("store: storage bucket not found")
	// ErrStorageBucketExists is returned when a bucket name or an app's env prefix is taken.
	ErrStorageBucketExists = errors.New("store: storage bucket already exists")
)

// StorageBucket is a named key-prefix slice of a storage destination.
type StorageBucket struct {
	ID            string
	Name          string
	TargetID      string
	Prefix        string
	Public        bool
	QuotaBytes    int64
	LifecycleDays int
	LastSweptAt   string
	CreatedAt     string
	// Purpose is StorageBucketPurposeApp or StorageBucketPurposeAssets.
	Purpose string
}

// Storage bucket purposes: app buckets bind to apps, asset buckets serve files (ADR 044).
const (
	StorageBucketPurposeApp    = "app"
	StorageBucketPurposeAssets = "assets"
)

// StorageBucketBinding links a bucket to an app and names the env var prefix it is injected under.
type StorageBucketBinding struct {
	Bucket    StorageBucket
	AppName   string
	EnvPrefix string
}

const storageBucketCols = `id, name, target_id, prefix, public, quota_bytes, lifecycle_days, last_swept_at, created_at, purpose`

func isStorageBucketConflict(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func scanStorageBucket(sc rowScanner) (StorageBucket, error) {
	var b StorageBucket
	var public int
	err := sc.Scan(&b.ID, &b.Name, &b.TargetID, &b.Prefix, &public, &b.QuotaBytes, &b.LifecycleDays, &b.LastSweptAt, &b.CreatedAt, &b.Purpose)
	b.Public = public != 0
	return b, err
}

// CreateStorageBucket inserts b, stamping CreatedAt.
func (db *DB) CreateStorageBucket(ctx context.Context, b StorageBucket) (StorageBucket, error) {
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if b.Purpose == "" {
		b.Purpose = StorageBucketPurposeApp
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_buckets (`+storageBucketCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, b.ID, b.Name, b.TargetID, b.Prefix, boolInt(b.Public), b.QuotaBytes, b.LifecycleDays, b.LastSweptAt, b.CreatedAt, b.Purpose)
	if err != nil {
		if isStorageBucketConflict(err) {
			return StorageBucket{}, ErrStorageBucketExists
		}
		return StorageBucket{}, fmt.Errorf("store: create storage bucket %q: %w", b.Name, err)
	}
	return b, nil
}

// GetStorageBucket returns one bucket by id.
func (db *DB) GetStorageBucket(ctx context.Context, id string) (StorageBucket, error) {
	b, err := scanStorageBucket(db.QueryRowContext(ctx, `SELECT `+storageBucketCols+` FROM storage_buckets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return StorageBucket{}, ErrStorageBucketNotFound
	}
	if err != nil {
		return StorageBucket{}, fmt.Errorf("store: get storage bucket %q: %w", id, err)
	}
	return b, nil
}

// ListStorageBuckets returns every bucket by name.
func (db *DB) ListStorageBuckets(ctx context.Context) ([]StorageBucket, error) {
	return db.listStorageBuckets(ctx, "")
}

// ListStorageBucketsByPurpose returns the buckets with one purpose by name.
func (db *DB) ListStorageBucketsByPurpose(ctx context.Context, purpose string) ([]StorageBucket, error) {
	return db.listStorageBuckets(ctx, purpose)
}

func (db *DB) listStorageBuckets(ctx context.Context, purpose string) ([]StorageBucket, error) {
	q := `SELECT ` + storageBucketCols + ` FROM storage_buckets`
	var args []any
	if purpose != "" {
		q += ` WHERE purpose = ?`
		args = append(args, purpose)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list storage buckets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StorageBucket
	for rows.Next() {
		b, err := scanStorageBucket(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan storage bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpdateStorageBucketSettings changes the public flag, quota and lifecycle of a bucket.
func (db *DB) UpdateStorageBucketSettings(ctx context.Context, id string, public bool, quotaBytes int64, lifecycleDays int) error {
	res, err := db.ExecContext(ctx, `UPDATE storage_buckets SET public = ?, quota_bytes = ?, lifecycle_days = ? WHERE id = ?`,
		boolInt(public), quotaBytes, lifecycleDays, id)
	if err != nil {
		return fmt.Errorf("store: update storage bucket %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStorageBucketNotFound
	}
	return nil
}

// MarkStorageBucketSwept records the time of the last lifecycle sweep.
func (db *DB) MarkStorageBucketSwept(ctx context.Context, id string, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE storage_buckets SET last_swept_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339), id); err != nil {
		return fmt.Errorf("store: mark storage bucket %q swept: %w", id, err)
	}
	return nil
}

// DeleteStorageBucket removes a bucket row and its bindings.
func (db *DB) DeleteStorageBucket(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM storage_bucket_bindings WHERE bucket_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete storage bucket bindings %q: %w", id, err)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM storage_buckets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete storage bucket %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStorageBucketNotFound
	}
	return nil
}

// BindStorageBucket links a bucket to an app under envPrefix, replacing that app's earlier link to the same bucket.
func (db *DB) BindStorageBucket(ctx context.Context, bucketID, appName, envPrefix string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_bucket_bindings (bucket_id, app_name, env_prefix, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (bucket_id, app_name) DO UPDATE SET env_prefix = excluded.env_prefix
	`, bucketID, appName, envPrefix, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if isStorageBucketConflict(err) {
			return ErrStorageBucketExists
		}
		return fmt.Errorf("store: bind storage bucket %q to %q: %w", bucketID, appName, err)
	}
	return nil
}

// UnbindStorageBucket removes an app's link to a bucket.
func (db *DB) UnbindStorageBucket(ctx context.Context, bucketID, appName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM storage_bucket_bindings WHERE bucket_id = ? AND app_name = ?`, bucketID, appName); err != nil {
		return fmt.Errorf("store: unbind storage bucket %q from %q: %w", bucketID, appName, err)
	}
	return nil
}

// ListStorageBucketBindings returns bindings for one app, or for every app when appName is empty.
func (db *DB) ListStorageBucketBindings(ctx context.Context, appName string) ([]StorageBucketBinding, error) {
	q := `SELECT b.id, b.name, b.target_id, b.prefix, b.public, b.quota_bytes, b.lifecycle_days, b.last_swept_at, b.created_at, b.purpose, l.app_name, l.env_prefix
		FROM storage_bucket_bindings l JOIN storage_buckets b ON b.id = l.bucket_id`
	var args []any
	if appName != "" {
		q += ` WHERE l.app_name = ?`
		args = append(args, appName)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY l.app_name, l.env_prefix`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list storage bucket bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StorageBucketBinding
	for rows.Next() {
		var l StorageBucketBinding
		var public int
		b := &l.Bucket
		if err := rows.Scan(&b.ID, &b.Name, &b.TargetID, &b.Prefix, &public, &b.QuotaBytes, &b.LifecycleDays, &b.LastSweptAt, &b.CreatedAt, &b.Purpose, &l.AppName, &l.EnvPrefix); err != nil {
			return nil, fmt.Errorf("store: scan storage bucket binding: %w", err)
		}
		b.Public = public != 0
		out = append(out, l)
	}
	return out, rows.Err()
}
