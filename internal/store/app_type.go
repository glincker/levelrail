package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for app types.
var (
	ErrAppTypeNotFound      = errors.New("store: app type not found")
	ErrAppTypeVersionExists = errors.New("store: app type version already published")
)

// AppTypeDraft is the editable document for one type.
type AppTypeDraft struct {
	Name      string
	Document  string
	UpdatedBy string
	UpdatedAt time.Time
}

// AppTypeVersion is one immutable published document.
type AppTypeVersion struct {
	Name        string
	Version     int
	Document    string
	Digest      string
	PublishedBy string
	PublishedAt time.Time
}

// AppTypeSummary is a list row: a draft plus its newest published version.
type AppTypeSummary struct {
	Name          string
	LatestVersion int
	UpdatedBy     string
	UpdatedAt     time.Time
}

// SaveAppTypeDraft creates or replaces a type's draft.
func (db *DB) SaveAppTypeDraft(ctx context.Context, name, document, by string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_types (name, draft, updated_by, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET draft = excluded.draft, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		name, document, by, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save app type %s: %w", name, err)
	}
	return nil
}

// GetAppTypeDraft returns a type's draft, or ErrAppTypeNotFound.
func (db *DB) GetAppTypeDraft(ctx context.Context, name string) (AppTypeDraft, error) {
	var d AppTypeDraft
	var at string
	err := db.QueryRowContext(ctx, `SELECT name, draft, updated_by, updated_at FROM app_types WHERE name = ?`, name).
		Scan(&d.Name, &d.Document, &d.UpdatedBy, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrAppTypeNotFound
	}
	if err != nil {
		return d, fmt.Errorf("store: get app type %s: %w", name, err)
	}
	d.UpdatedAt = parseAppTypeTime(at)
	return d, nil
}

// ListAppTypes returns every type with its newest published version (0 when none).
func (db *DB) ListAppTypes(ctx context.Context) ([]AppTypeSummary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.name, COALESCE((SELECT MAX(version) FROM app_type_versions v WHERE v.name = t.name), 0), t.updated_by, t.updated_at
		FROM app_types t ORDER BY t.name`)
	if err != nil {
		return nil, fmt.Errorf("store: list app types: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppTypeSummary
	for rows.Next() {
		var s AppTypeSummary
		var at string
		if err := rows.Scan(&s.Name, &s.LatestVersion, &s.UpdatedBy, &at); err != nil {
			return nil, fmt.Errorf("store: scan app type: %w", err)
		}
		s.UpdatedAt = parseAppTypeTime(at)
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetAppTypeVersion returns one published version; version 0 means the newest.
func (db *DB) GetAppTypeVersion(ctx context.Context, name string, version int) (AppTypeVersion, error) {
	q := `SELECT name, version, document, digest, published_by, published_at FROM app_type_versions WHERE name = ? `
	args := []any{name}
	if version > 0 {
		q += `AND version = ?`
		args = append(args, version)
	} else {
		q += `ORDER BY version DESC LIMIT 1`
	}
	var v AppTypeVersion
	var at string
	err := db.QueryRowContext(ctx, q, args...).Scan(&v.Name, &v.Version, &v.Document, &v.Digest, &v.PublishedBy, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrAppTypeNotFound
	}
	if err != nil {
		return v, fmt.Errorf("store: get app type version %s@%d: %w", name, version, err)
	}
	v.PublishedAt = parseAppTypeTime(at)
	return v, nil
}

// CountAppTypeVersions reports how many versions a type has published.
func (db *DB) CountAppTypeVersions(ctx context.Context, name string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_type_versions WHERE name = ?`, name).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count app type versions %s: %w", name, err)
	}
	return n, nil
}

// InsertAppTypeVersion stores a published version. The primary key makes a
// repeat of the same number fail, which is what keeps versions immutable.
func (db *DB) InsertAppTypeVersion(ctx context.Context, v AppTypeVersion) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_type_versions (name, version, document, digest, published_by, published_at) VALUES (?, ?, ?, ?, ?, ?)`,
		v.Name, v.Version, v.Document, v.Digest, v.PublishedBy, formatTime(time.Now()))
	if isUniqueViolation(err) {
		return ErrAppTypeVersionExists
	}
	if err != nil {
		return fmt.Errorf("store: publish app type %s@%d: %w", v.Name, v.Version, err)
	}
	return nil
}

func parseAppTypeTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
