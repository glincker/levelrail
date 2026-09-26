package cpbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Prefix is the top-level key prefix for off-box control plane backups.
const Prefix = "cp-backups"

const (
	dataSuffix     = ".db.age"
	manifestSuffix = ".json"
	manifestFormat = 1
)

// Manifest describes one off-box backup. It is stored unencrypted beside the
// ciphertext and holds no key material.
type Manifest struct {
	ManifestVersion int       `json:"manifest_version"`
	InstallID       string    `json:"install_id"`
	Key             string    `json:"key"`
	CreatedAt       time.Time `json:"created_at"`
	BinaryVersion   string    `json:"binary_version"`
	SchemaVersion   int       `json:"schema_version"`
	// MigrationsApplied is the row count of the snapshot's schema_migrations.
	MigrationsApplied int    `json:"migrations_applied"`
	SHA256            string `json:"sha256"`
	SizeBytes         int64  `json:"size_bytes"`
	PlainSHA256       string `json:"plain_sha256"`
	PlainSizeBytes    int64  `json:"plain_size_bytes"`
	Compression       string `json:"compression"`
	Encryption        string `json:"encryption"`
	RecipientCount    int    `json:"recipient_count"`
	// ContainsWrappedSecrets is true: the database holds secret ciphertexts and
	// wrapped data keys. IncludesMasterKey is always false.
	ContainsWrappedSecrets bool `json:"contains_wrapped_secrets"`
	IncludesMasterKey      bool `json:"includes_master_key"`
}

var keyPattern = regexp.MustCompile(`^` + Prefix + `/([^/]+)/(\d{4})/(\d{2})/(\d{2})/(\d{8}T\d{6}Z)\.db\.age$`)

// DataKey is the ciphertext object key for a backup taken at ts.
func DataKey(installID string, ts time.Time) string {
	ts = ts.UTC()
	return fmt.Sprintf("%s/%s/%s/%s%s", Prefix, installID, ts.Format("2006/01/02"), ts.Format(nameLayout), dataSuffix)
}

// ManifestKey is the manifest object key beside a ciphertext key.
func ManifestKey(dataKey string) string {
	return strings.TrimSuffix(dataKey, dataSuffix) + manifestSuffix
}

// ParseKey extracts the install id and timestamp from a ciphertext key.
func ParseKey(key string) (installID string, ts time.Time, err error) {
	m := keyPattern.FindStringSubmatch(key)
	if m == nil {
		return "", time.Time{}, fmt.Errorf("not a control plane backup key: %q", key)
	}
	ts, err = time.Parse(nameLayout, m[5])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse backup key time: %w", err)
	}
	return m[1], ts.UTC(), nil
}

// ErrBadManifest means a manifest is unreadable or self-inconsistent.
var ErrBadManifest = errors.New("invalid backup manifest")

// ParseManifest decodes and sanity-checks a manifest.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	switch {
	case m.ManifestVersion != manifestFormat:
		return Manifest{}, fmt.Errorf("%w: unsupported manifest version %d", ErrBadManifest, m.ManifestVersion)
	case m.SHA256 == "" || m.PlainSHA256 == "" || m.SizeBytes <= 0 || m.PlainSizeBytes <= 0:
		return Manifest{}, fmt.Errorf("%w: missing checksum or size", ErrBadManifest)
	case m.InstallID == "" || m.Key == "":
		return Manifest{}, fmt.Errorf("%w: missing install id or key", ErrBadManifest)
	}
	return m, nil
}

func inspectMigrationCount(ctx context.Context, path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count applied migrations: %w", err)
	}
	return n, nil
}
