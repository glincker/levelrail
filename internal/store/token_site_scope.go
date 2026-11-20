package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrInvalidSiteScope rejects a site scope without a token or a site.
var ErrInvalidSiteScope = errors.New("store: invalid token site scope")

// APITokenSiteScope binds a token to one Pages site.
type APITokenSiteScope struct {
	TokenID  string
	SiteName string
}

// SetAPITokenSiteScope records a token's site scope.
func (db *DB) SetAPITokenSiteScope(ctx context.Context, s APITokenSiteScope) error {
	if s.TokenID == "" || s.SiteName == "" {
		return ErrInvalidSiteScope
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO api_token_site_scopes (token_id, site_name) VALUES (?, ?)
		ON CONFLICT(token_id) DO UPDATE SET site_name = excluded.site_name`, s.TokenID, s.SiteName)
	if err != nil {
		return fmt.Errorf("store: set token site scope: %w", err)
	}
	return nil
}

// GetAPITokenSiteScope returns a token's site scope, or nil when it has none.
func (db *DB) GetAPITokenSiteScope(ctx context.Context, tokenID string) (*APITokenSiteScope, error) {
	s := APITokenSiteScope{TokenID: tokenID}
	err := db.QueryRowContext(ctx, `SELECT site_name FROM api_token_site_scopes WHERE token_id = ?`, tokenID).Scan(&s.SiteName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get token site scope: %w", err)
	}
	if s.SiteName == "" {
		return nil, ErrInvalidSiteScope
	}
	return &s, nil
}

// ListAPITokenIDsForSite returns the ids of every token scoped to a site.
func (db *DB) ListAPITokenIDsForSite(ctx context.Context, site string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT token_id FROM api_token_site_scopes WHERE site_name = ? ORDER BY token_id`, site)
	if err != nil {
		return nil, fmt.Errorf("store: list site tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan site token: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
