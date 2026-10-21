package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// heldStatic is the desired_json shape of a held static build, which holds a
// site instead of a desired service.
type heldStatic struct {
	Site *StaticSite `json:"static_site"`
}

func heldStaticSite(raw string) *StaticSite {
	var h heldStatic
	if json.Unmarshal([]byte(raw), &h) != nil {
		return nil
	}
	return h.Site
}

// SaveHeldStaticBuild records the site a finished static build would have
// served, keyed by its deploy attempt.
func (db *DB) SaveHeldStaticBuild(ctx context.Context, deployID string, site StaticSite, at time.Time) error {
	raw, err := json.Marshal(heldStatic{Site: &site})
	if err != nil {
		return fmt.Errorf("store: marshal held static build %q: %w", deployID, err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO held_builds (deploy_id, service_name, desired_json, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (deploy_id) DO UPDATE SET desired_json = excluded.desired_json
	`, deployID, site.Name, string(raw), at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: save held static build %q: %w", deployID, err)
	}
	return nil
}

// GetHeldStaticBuild returns the unpublished held static build's site, or
// ErrHeldBuildNotFound when the id is unknown, published or not a static build.
func (db *DB) GetHeldStaticBuild(ctx context.Context, deployID string) (*StaticSite, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT desired_json FROM held_builds WHERE deploy_id = ? AND published_at IS NULL`, deployID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHeldBuildNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get held static build %q: %w", deployID, err)
	}
	site := heldStaticSite(raw)
	if site == nil {
		return nil, ErrHeldBuildNotFound
	}
	return site, nil
}
