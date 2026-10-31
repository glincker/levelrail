package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	TenantID           string
	SSOPolicy          OAuthSSOPolicy
}

// OAuthSSOPolicy is the SAML-style sign-in policy of one OAuth or OIDC
// provider (migration 2360). The zero value is the least privilege setting.
type OAuthSSOPolicy struct {
	GroupsClaim     string
	SyncRoles       bool
	DefaultRoleID   string
	JITProvisioning bool
	AllowedDomains  []string
}

const oauthSettingsColumns = `provider, enabled, client_id, allowed_email_domain, issuer_url, display_name,
	groups_claim, sync_roles, default_role_id, jit_provisioning, allowed_domains, tenant_id`

func scanOAuthSettings(scan func(dest ...any) error) (OAuthProviderSettings, error) {
	var (
		s                    OAuthProviderSettings
		enabled, sync, jit   int
		clientID, allowedDom sql.NullString
		issuerURL, dispName  sql.NullString
		domains              string
	)
	p := &s.SSOPolicy
	if err := scan(&s.Provider, &enabled, &clientID, &allowedDom, &issuerURL, &dispName,
		&p.GroupsClaim, &sync, &p.DefaultRoleID, &jit, &domains, &s.TenantID); err != nil {
		return OAuthProviderSettings{}, err
	}
	if err := json.Unmarshal([]byte(domains), &p.AllowedDomains); err != nil {
		return OAuthProviderSettings{}, fmt.Errorf("allowed_domains: %w", err)
	}
	s.Enabled, p.SyncRoles, p.JITProvisioning = enabled != 0, sync != 0, jit != 0
	s.ClientID, s.AllowedEmailDomain = clientID.String, allowedDom.String
	s.IssuerURL, s.DisplayName = issuerURL.String, dispName.String
	return s, nil
}

// ErrOAuthProviderNotFound is returned when no settings row has the given provider.
var ErrOAuthProviderNotFound = errors.New("store: oauth provider not found")

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
	s, err := scanOAuthSettings(db.QueryRowContext(ctx, `SELECT `+oauthSettingsColumns+` FROM oauth_provider_settings WHERE provider = ?`, provider).Scan)
	if err != nil {
		return OAuthProviderSettings{}, fmt.Errorf("store: get oauth provider settings %q: %w", provider, err)
	}
	return s, nil
}

// ListOAuthProviderSettings returns every provider row, ordered by
// provider name: GET /api/v1/settings/oauth's read model.
func (db *DB) ListOAuthProviderSettings(ctx context.Context) ([]OAuthProviderSettings, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+oauthSettingsColumns+` FROM oauth_provider_settings ORDER BY provider`)
	if err != nil {
		return nil, fmt.Errorf("store: list oauth provider settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []OAuthProviderSettings
	for rows.Next() {
		s, err := scanOAuthSettings(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan oauth provider settings row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate oauth provider settings rows: %w", err)
	}
	return out, nil
}

// UpdateOAuthProviderSettings replaces one provider's client settings. It
// never touches a credential or the SSO policy (see SetOAuthSSOPolicy).
func (db *DB) UpdateOAuthProviderSettings(ctx context.Context, s OAuthProviderSettings) error {
	if !IsOAuthProvider(s.Provider) {
		return fmt.Errorf("%w: %q", ErrUnknownOAuthProvider, s.Provider)
	}
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE oauth_provider_settings
		SET enabled = ?, client_id = ?, allowed_email_domain = ?, issuer_url = ?, display_name = ?, tenant_id = ?
		WHERE provider = ?
	`,
		enabled,
		sql.NullString{String: s.ClientID, Valid: s.ClientID != ""},
		sql.NullString{String: s.AllowedEmailDomain, Valid: s.AllowedEmailDomain != ""},
		sql.NullString{String: s.IssuerURL, Valid: s.IssuerURL != ""},
		sql.NullString{String: s.DisplayName, Valid: s.DisplayName != ""},
		s.TenantID,
		s.Provider,
	)
	if err != nil {
		return fmt.Errorf("store: update oauth provider settings %q: %w", s.Provider, err)
	}
	return nil
}

// SetOAuthSSOPolicy replaces one provider's SSO policy.
func (db *DB) SetOAuthSSOPolicy(ctx context.Context, provider string, p OAuthSSOPolicy) error {
	domains, err := json.Marshal(nonNilSlice(p.AllowedDomains))
	if err != nil {
		return fmt.Errorf("store: marshal oauth domains: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE oauth_provider_settings SET groups_claim = ?, sync_roles = ?,
		default_role_id = ?, jit_provisioning = ?, allowed_domains = ? WHERE provider = ?`,
		p.GroupsClaim, boolInt(p.SyncRoles), p.DefaultRoleID, boolInt(p.JITProvisioning), string(domains), provider)
	if err != nil {
		return fmt.Errorf("store: set oauth sso policy %q: %w", provider, err)
	}
	return rowsAffectedOrNotFound(res, ErrOAuthProviderNotFound, "set oauth sso policy %q", provider)
}
