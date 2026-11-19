package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// StaticSiteSettings are the per-site overrides edited in the dashboard. A nil
// pointer or empty string means "no override, keep what app.yaml says".
type StaticSiteSettings struct {
	BuildCommand  string `json:"build_command"`
	OutputDir     string `json:"output_dir"`
	SPAFallback   *bool  `json:"spa_fallback"`
	CleanURLs     *bool  `json:"clean_urls"`
	TrailingSlash string `json:"trailing_slash"`
	NotFoundPage  string `json:"not_found_page"`
}

// StaticUIRedirect is one redirect rule edited in the dashboard.
type StaticUIRedirect struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// StaticUIHeader is one header block edited in the dashboard.
type StaticUIHeader struct {
	Path   string            `json:"path"`
	Set    map[string]string `json:"set,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

// StaticUIRules is the ruleset stored on a site; it ranks before app.yaml and the files.
type StaticUIRules struct {
	Redirects []StaticUIRedirect `json:"redirects"`
	Headers   []StaticUIHeader   `json:"headers"`
}

// GetStaticSiteSettings returns the stored overrides; a site with none yields zero values.
func (db *DB) GetStaticSiteSettings(ctx context.Context, name string) (StaticSiteSettings, StaticUIRules, error) {
	var s StaticSiteSettings
	var r StaticUIRules
	var sj, rj string
	err := db.QueryRowContext(ctx, `SELECT settings, ui_rules FROM static_site_settings WHERE name = ?`, name).Scan(&sj, &rj)
	if errors.Is(err, sql.ErrNoRows) {
		return s, StaticUIRules{Redirects: []StaticUIRedirect{}, Headers: []StaticUIHeader{}}, nil
	}
	if err != nil {
		return s, r, fmt.Errorf("store: get static site settings %q: %w", name, err)
	}
	if err := json.Unmarshal([]byte(sj), &s); err != nil {
		return s, r, fmt.Errorf("store: decode static site settings %q: %w", name, err)
	}
	if err := json.Unmarshal([]byte(rj), &r); err != nil {
		return s, r, fmt.Errorf("store: decode static site rules %q: %w", name, err)
	}
	if r.Redirects == nil {
		r.Redirects = []StaticUIRedirect{}
	}
	if r.Headers == nil {
		r.Headers = []StaticUIHeader{}
	}
	return s, r, nil
}

// SaveStaticSiteSettings replaces the settings, keeping any stored rules.
func (db *DB) SaveStaticSiteSettings(ctx context.Context, name string, s StaticSiteSettings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("store: encode static site settings %q: %w", name, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO static_site_settings (name, settings) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET settings = excluded.settings, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, name, string(raw))
	if err != nil {
		return fmt.Errorf("store: save static site settings %q: %w", name, err)
	}
	return nil
}

// SaveStaticSiteUIRules replaces the stored rules, keeping any stored settings.
func (db *DB) SaveStaticSiteUIRules(ctx context.Context, name string, r StaticUIRules) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("store: encode static site rules %q: %w", name, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO static_site_settings (name, ui_rules) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET ui_rules = excluded.ui_rules, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, name, string(raw))
	if err != nil {
		return fmt.Errorf("store: save static site rules %q: %w", name, err)
	}
	return nil
}

// DeleteStaticSiteSettings drops both; a missing row is not an error.
func (db *DB) DeleteStaticSiteSettings(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM static_site_settings WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: delete static site settings %q: %w", name, err)
	}
	return nil
}
