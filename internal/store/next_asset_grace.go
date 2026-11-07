package store

import (
	"context"
	"fmt"
	"time"
)

// NextAssetGrace is a superseded Next.js release that keeps serving its old
// static chunks and data requests until Until (migrations/3202_next_asset_grace.sql).
type NextAssetGrace struct {
	Service      string
	DeploymentID string
	Image        string
	Until        time.Time
}

// InsertNextAssetGrace records a grace window; a repeat of the same release replaces it.
func (db *DB) InsertNextAssetGrace(ctx context.Context, g NextAssetGrace) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO next_asset_grace (service_name, deployment_id, image, until)
		VALUES (?, ?, ?, ?)`,
		g.Service, g.DeploymentID, g.Image, g.Until.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: insert next asset grace: %w", err)
	}
	return nil
}

// ListActiveNextAssetGrace returns windows that end after now, soonest first.
func (db *DB) ListActiveNextAssetGrace(ctx context.Context, now time.Time) ([]NextAssetGrace, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, deployment_id, image, until FROM next_asset_grace
		WHERE until > ? ORDER BY until ASC`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("store: list next asset grace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NextAssetGrace
	for rows.Next() {
		var g NextAssetGrace
		var until string
		if err := rows.Scan(&g.Service, &g.DeploymentID, &g.Image, &until); err != nil {
			return nil, fmt.Errorf("store: scan next asset grace: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, until)
		if err != nil {
			continue
		}
		g.Until = t
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteExpiredNextAssetGrace drops windows that ended at or before now.
func (db *DB) DeleteExpiredNextAssetGrace(ctx context.Context, now time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM next_asset_grace WHERE until <= ?`, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: delete expired next asset grace: %w", err)
	}
	return nil
}

// DeleteNextAssetGraceForService drops every window of one service, used when the cap evicts older generations.
func (db *DB) DeleteNextAssetGraceForService(ctx context.Context, service string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM next_asset_grace WHERE service_name = ?`, service); err != nil {
		return fmt.Errorf("store: delete next asset grace: %w", err)
	}
	return nil
}
