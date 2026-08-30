package store

import (
	"context"
	"database/sql"
	"fmt"
)

// OAuthProviderSettings is one row of oauth_provider_settings. IssuerURL
// and DisplayName are only meaningful for the oidc provider.
type OAuthProviderSettings struct {
	Provider           string
	Enabled            bool
	ClientID           string
	AllowedEmailDomain string
	IssuerURL          string
	DisplayName        string
}

// OAuthProviderSecretsKey is the internal/secrets serviceName a
// provider's client secret is stored under, mirroring
// BackupTargetSecretsKey's shape.
func OAuthProviderSecretsKey(provider string) string {
	return "oauth-provider/" + provider
}

// OAuthProviderSecretEnvKey is the fixed envKey used within that namespace.
const OAuthProviderSecretEnvKey = "client_secret"

// GetOAuthProviderSettings returns the settings row for provider.
func (db *DB) GetOAuthProviderSettings(ctx context.Context, provider string) (OAuthProviderSettings, error) {
	var (
		s          OAuthProviderSettings
		enabled    int
		clientID   sql.NullString
		allowedDom sql.NullString
		issuerURL  sql.NullString
		dispName   sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT provider, enabled, client_id, allowed_email_domain, issuer_url, display_name
		FROM oauth_provider_settings WHERE provider = ?
	`, provider).Scan(&s.Provider, &enabled, &clientID, &allowedDom, &issuerURL, &dispName)
	if err != nil {
		return OAuthProviderSettings{}, fmt.Errorf("store: get oauth provider settings %q: %w", provider, err)
	}
	s.Enabled = enabled != 0
	s.ClientID = clientID.String
	s.AllowedEmailDomain = allowedDom.String
	s.IssuerURL = issuerURL.String
	s.DisplayName = dispName.String
	return s, nil
}

// ListOAuthProviderSettings returns both provider rows, ordered by
// provider name: GET /api/v1/settings/oauth's read model.
func (db *DB) ListOAuthProviderSettings(ctx context.Context) ([]OAuthProviderSettings, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT provider, enabled, client_id, allowed_email_domain, issuer_url, display_name
		FROM oauth_provider_settings ORDER BY provider
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list oauth provider settings: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []OAuthProviderSettings
	for rows.Next() {
		var (
			s          OAuthProviderSettings
			enabled    int
			clientID   sql.NullString
			allowedDom sql.NullString
			issuerURL  sql.NullString
			dispName   sql.NullString
		)
		if err := rows.Scan(&s.Provider, &enabled, &clientID, &allowedDom, &issuerURL, &dispName); err != nil {
			return nil, fmt.Errorf("store: scan oauth provider settings row: %w", err)
		}
		s.Enabled = enabled != 0
		s.ClientID = clientID.String
		s.AllowedEmailDomain = allowedDom.String
		s.IssuerURL = issuerURL.String
		s.DisplayName = dispName.String
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate oauth provider settings rows: %w", err)
	}
	return out, nil
}

// UpdateOAuthProviderSettings replaces one provider's settings row in
// full. It never touches a credential; storing the client secret is the
// caller's job via internal/secrets.
func (db *DB) UpdateOAuthProviderSettings(ctx context.Context, s OAuthProviderSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE oauth_provider_settings
		SET enabled = ?, client_id = ?, allowed_email_domain = ?, issuer_url = ?, display_name = ?
		WHERE provider = ?
	`,
		enabled,
		sql.NullString{String: s.ClientID, Valid: s.ClientID != ""},
		sql.NullString{String: s.AllowedEmailDomain, Valid: s.AllowedEmailDomain != ""},
		sql.NullString{String: s.IssuerURL, Valid: s.IssuerURL != ""},
		sql.NullString{String: s.DisplayName, Valid: s.DisplayName != ""},
		s.Provider,
	)
	if err != nil {
		return fmt.Errorf("store: update oauth provider settings %q: %w", s.Provider, err)
	}
	return nil
}
