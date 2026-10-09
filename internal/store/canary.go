package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CanaryServiceSuffix marks the hidden clone service that runs a canary.
const CanaryServiceSuffix = "--canary"

// CanaryServiceName returns the hidden service name that runs serviceName's canary.
func CanaryServiceName(serviceName string) string { return serviceName + CanaryServiceSuffix }

// IsCanaryService reports whether name is a hidden canary clone.
func IsCanaryService(name string) bool { return strings.HasSuffix(name, CanaryServiceSuffix) }

// CanaryRelease is an app's in-flight canary (migrations/0378_canary_releases.sql).
type CanaryRelease struct {
	ServiceName   string
	CanaryService string
	Image         string
	Weight        int
	CreatedAt     time.Time
}

// SetCanaryRelease creates or replaces serviceName's canary row.
func (db *DB) SetCanaryRelease(ctx context.Context, c CanaryRelease) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canary_releases (service_name, canary_service, image, weight, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET canary_service = excluded.canary_service, image = excluded.image, weight = excluded.weight
	`, c.ServiceName, c.CanaryService, c.Image, c.Weight, c.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set canary release for %q: %w", c.ServiceName, err)
	}
	return nil
}

// GetCanaryRelease returns serviceName's canary and whether one exists.
func (db *DB) GetCanaryRelease(ctx context.Context, serviceName string) (CanaryRelease, bool, error) {
	var c CanaryRelease
	var created string
	err := db.QueryRowContext(ctx, `
		SELECT service_name, canary_service, image, weight, created_at FROM canary_releases WHERE service_name = ?
	`, serviceName).Scan(&c.ServiceName, &c.CanaryService, &c.Image, &c.Weight, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return CanaryRelease{}, false, nil
	}
	if err != nil {
		return CanaryRelease{}, false, fmt.Errorf("store: get canary release for %q: %w", serviceName, err)
	}
	if c.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return CanaryRelease{}, false, fmt.Errorf("store: get canary release for %q: %w", serviceName, err)
	}
	return c, true, nil
}

// DeleteCanaryRelease removes serviceName's canary row; a missing row is not an error.
func (db *DB) DeleteCanaryRelease(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM canary_releases WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete canary release for %q: %w", serviceName, err)
	}
	return nil
}

// ListCanaryReleases returns every in-flight canary.
func (db *DB) ListCanaryReleases(ctx context.Context) ([]CanaryRelease, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, canary_service, image, weight, created_at FROM canary_releases ORDER BY service_name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list canary releases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CanaryRelease
	for rows.Next() {
		var c CanaryRelease
		var created string
		if err := rows.Scan(&c.ServiceName, &c.CanaryService, &c.Image, &c.Weight, &created); err != nil {
			return nil, fmt.Errorf("store: list canary releases: %w", err)
		}
		if c.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, fmt.Errorf("store: list canary releases: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
