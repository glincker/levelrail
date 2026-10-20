package store

import (
	"context"
	"fmt"
	"strings"
)

// ExposureRestriction is a desired DOCKER-USER allow-list for one
// published port of the control plane's own host.
type ExposureRestriction struct {
	Port      int
	Protocol  string
	Allow     []string
	CreatedBy string
	CreatedAt string
}

// SaveExposureRestriction inserts or replaces the restriction for a port.
func (db *DB) SaveExposureRestriction(ctx context.Context, r ExposureRestriction) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO exposure_restrictions (port, protocol, allow, created_by, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (port, protocol) DO UPDATE SET allow = excluded.allow, created_by = excluded.created_by
	`, r.Port, r.Protocol, strings.Join(r.Allow, ","), r.CreatedBy, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save exposure restriction %d/%s: %w", r.Port, r.Protocol, err)
	}
	return nil
}

// ListExposureRestrictions returns every restriction, by port.
func (db *DB) ListExposureRestrictions(ctx context.Context) ([]ExposureRestriction, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT port, protocol, allow, created_by, created_at FROM exposure_restrictions ORDER BY port, protocol
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list exposure restrictions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ExposureRestriction
	for rows.Next() {
		var r ExposureRestriction
		var allow string
		if err := rows.Scan(&r.Port, &r.Protocol, &allow, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan exposure restriction row: %w", err)
		}
		if allow != "" {
			r.Allow = strings.Split(allow, ",")
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate exposure restriction rows: %w", err)
	}
	return out, nil
}

// DeleteExposureRestriction removes a restriction; a missing row is not an error.
func (db *DB) DeleteExposureRestriction(ctx context.Context, port int, protocol string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM exposure_restrictions WHERE port = ? AND protocol = ?`, port, protocol); err != nil {
		return fmt.Errorf("store: delete exposure restriction %d/%s: %w", port, protocol, err)
	}
	return nil
}
