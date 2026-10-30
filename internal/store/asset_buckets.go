package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Asset bucket visibility and serving modes (ADR 044).
const (
	AssetVisibilityPrivate = "private"
	AssetVisibilityPublic  = "public"

	AssetServingDirect  = "direct"
	AssetServingProxied = "proxied"
	AssetServingCDN     = "cdn"
)

var (
	// ErrAssetBucketNotFound is returned when a bucket has no asset settings row.
	ErrAssetBucketNotFound = errors.New("store: asset bucket not found")
	// ErrAssetBucketInvalid is returned for an unknown visibility or serving mode.
	ErrAssetBucketInvalid = errors.New("store: invalid asset bucket settings")
)

// AssetBucketSettings is the sidecar of a storage bucket with purpose 'assets'.
type AssetBucketSettings struct {
	BucketID        string
	ProjectID       string
	Visibility      string
	ServingMode     string
	PublicBaseURL   string
	CDNBaseURL      string
	ReferrerAllow   []string
	RefuseNoReferer bool
	UpdatedAt       string
}

// AssetBucket joins a storage bucket with its asset settings.
type AssetBucket struct {
	Bucket   StorageBucket
	Settings AssetBucketSettings
}

const assetSettingsCols = `bucket_id, COALESCE(project_id, ''), visibility, serving_mode, public_base_url, cdn_base_url, referrer_allow, refuse_no_referer, updated_at`

// normalize fills defaults (private, direct) and rejects unknown values.
func (s *AssetBucketSettings) normalize() error {
	if s.Visibility == "" {
		s.Visibility = AssetVisibilityPrivate
	}
	if s.ServingMode == "" {
		s.ServingMode = AssetServingDirect
	}
	switch s.Visibility {
	case AssetVisibilityPrivate, AssetVisibilityPublic:
	default:
		return fmt.Errorf("%w: visibility %q", ErrAssetBucketInvalid, s.Visibility)
	}
	switch s.ServingMode {
	case AssetServingDirect, AssetServingProxied, AssetServingCDN:
	default:
		return fmt.Errorf("%w: serving mode %q", ErrAssetBucketInvalid, s.ServingMode)
	}
	return nil
}

func scanAssetSettings(sc rowScanner) (AssetBucketSettings, error) {
	var s AssetBucketSettings
	var allow string
	var refuse int
	if err := sc.Scan(&s.BucketID, &s.ProjectID, &s.Visibility, &s.ServingMode, &s.PublicBaseURL, &s.CDNBaseURL, &allow, &refuse, &s.UpdatedAt); err != nil {
		return AssetBucketSettings{}, err
	}
	s.RefuseNoReferer = refuse != 0
	if err := json.Unmarshal([]byte(allow), &s.ReferrerAllow); err != nil {
		return AssetBucketSettings{}, fmt.Errorf("decode referrer allowlist: %w", err)
	}
	return s, nil
}

func encodeAllow(hosts []string) (string, error) {
	if hosts == nil {
		hosts = []string{}
	}
	b, err := json.Marshal(hosts)
	if err != nil {
		return "", fmt.Errorf("encode referrer allowlist: %w", err)
	}
	return string(b), nil
}

func nullableProject(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// CreateAssetBucket inserts the storage bucket (purpose forced to assets) and its settings in one transaction.
func (db *DB) CreateAssetBucket(ctx context.Context, b StorageBucket, s AssetBucketSettings) (AssetBucket, error) {
	b.Purpose = StorageBucketPurposeAssets
	s.BucketID = b.ID
	if err := s.normalize(); err != nil {
		return AssetBucket{}, err
	}
	allow, err := encodeAllow(s.ReferrerAllow)
	if err != nil {
		return AssetBucket{}, err
	}
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.UpdatedAt = b.CreatedAt
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return AssetBucket{}, fmt.Errorf("store: begin create asset bucket: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO storage_buckets (`+storageBucketCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, b.ID, b.Name, b.TargetID, b.Prefix, boolInt(b.Public), b.QuotaBytes, b.LifecycleDays, b.LastSweptAt, b.CreatedAt, b.Purpose)
	if err != nil {
		if isStorageBucketConflict(err) {
			return AssetBucket{}, ErrStorageBucketExists
		}
		return AssetBucket{}, fmt.Errorf("store: create asset bucket %q: %w", b.Name, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO asset_bucket_settings (bucket_id, project_id, visibility, serving_mode, public_base_url, cdn_base_url, referrer_allow, refuse_no_referer, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.BucketID, nullableProject(s.ProjectID), s.Visibility, s.ServingMode, s.PublicBaseURL, s.CDNBaseURL, allow, boolInt(s.RefuseNoReferer), s.UpdatedAt)
	if err != nil {
		return AssetBucket{}, fmt.Errorf("store: create asset bucket settings %q: %w", b.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return AssetBucket{}, fmt.Errorf("store: commit asset bucket %q: %w", b.Name, err)
	}
	return AssetBucket{Bucket: b, Settings: s}, nil
}

// GetAssetBucket returns an asset bucket with its settings, or ErrAssetBucketNotFound.
func (db *DB) GetAssetBucket(ctx context.Context, id string) (AssetBucket, error) {
	b, err := db.GetStorageBucket(ctx, id)
	if errors.Is(err, ErrStorageBucketNotFound) {
		return AssetBucket{}, ErrAssetBucketNotFound
	}
	if err != nil {
		return AssetBucket{}, err
	}
	if b.Purpose != StorageBucketPurposeAssets {
		return AssetBucket{}, ErrAssetBucketNotFound
	}
	s, err := scanAssetSettings(db.QueryRowContext(ctx, `SELECT `+assetSettingsCols+` FROM asset_bucket_settings WHERE bucket_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return AssetBucket{}, ErrAssetBucketNotFound
	}
	if err != nil {
		return AssetBucket{}, fmt.Errorf("store: get asset bucket settings %q: %w", id, err)
	}
	return AssetBucket{Bucket: b, Settings: s}, nil
}

// ListAssetBuckets returns asset buckets by name; a non-empty projectID limits the list to that project.
func (db *DB) ListAssetBuckets(ctx context.Context, projectID string) ([]AssetBucket, error) {
	q := `SELECT b.id, b.name, b.target_id, b.prefix, b.public, b.quota_bytes, b.lifecycle_days, b.last_swept_at, b.created_at, b.purpose,
		s.bucket_id, COALESCE(s.project_id, ''), s.visibility, s.serving_mode, s.public_base_url, s.cdn_base_url, s.referrer_allow, s.refuse_no_referer, s.updated_at
		FROM storage_buckets b JOIN asset_bucket_settings s ON s.bucket_id = b.id WHERE b.purpose = ?`
	args := []any{StorageBucketPurposeAssets}
	if projectID != "" {
		q += ` AND s.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY b.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list asset buckets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AssetBucket
	for rows.Next() {
		var ab AssetBucket
		var public, refuse int
		var allow string
		b, s := &ab.Bucket, &ab.Settings
		if err := rows.Scan(&b.ID, &b.Name, &b.TargetID, &b.Prefix, &public, &b.QuotaBytes, &b.LifecycleDays, &b.LastSweptAt, &b.CreatedAt, &b.Purpose,
			&s.BucketID, &s.ProjectID, &s.Visibility, &s.ServingMode, &s.PublicBaseURL, &s.CDNBaseURL, &allow, &refuse, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan asset bucket: %w", err)
		}
		b.Public = public != 0
		s.RefuseNoReferer = refuse != 0
		if err := json.Unmarshal([]byte(allow), &s.ReferrerAllow); err != nil {
			return nil, fmt.Errorf("store: decode referrer allowlist for %q: %w", b.ID, err)
		}
		out = append(out, ab)
	}
	return out, rows.Err()
}

// UpdateAssetBucketSettings replaces the settings of an existing asset bucket and mirrors visibility onto storage_buckets.public.
func (db *DB) UpdateAssetBucketSettings(ctx context.Context, s AssetBucketSettings) error {
	if err := s.normalize(); err != nil {
		return err
	}
	allow, err := encodeAllow(s.ReferrerAllow)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin update asset bucket: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE asset_bucket_settings SET project_id = ?, visibility = ?, serving_mode = ?, public_base_url = ?, cdn_base_url = ?,
			referrer_allow = ?, refuse_no_referer = ?, updated_at = ?
		WHERE bucket_id = ?
	`, nullableProject(s.ProjectID), s.Visibility, s.ServingMode, s.PublicBaseURL, s.CDNBaseURL, allow, boolInt(s.RefuseNoReferer),
		time.Now().UTC().Format(time.RFC3339), s.BucketID)
	if err != nil {
		return fmt.Errorf("store: update asset bucket settings %q: %w", s.BucketID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAssetBucketNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE storage_buckets SET public = ? WHERE id = ?`, boolInt(s.Visibility == AssetVisibilityPublic), s.BucketID); err != nil {
		return fmt.Errorf("store: mirror asset bucket visibility %q: %w", s.BucketID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit asset bucket update %q: %w", s.BucketID, err)
	}
	return nil
}
