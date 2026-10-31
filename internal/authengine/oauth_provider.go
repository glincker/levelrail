package authengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	theauth "github.com/glincker/theauth-go/v2"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultGraphMeURL = "https://graph.microsoft.com/v1.0/me"
	maxUserInfoBody   = 1 << 16
)

var (
	errUserInfoIncomplete = errors.New("authengine: provider returned no subject or email")
	errEmailInUse         = errors.New("authengine: email belongs to an existing account")
	errDomainNotAllowed   = errors.New("authengine: email domain not allowed")
	errNotProvisioned     = errors.New("authengine: no account for this identity and provisioning is off")
	errTenantRequired     = fmt.Errorf("%w: set a tenant id on the Microsoft provider to allow sign-up", errNotProvisioned)
	errTenantMismatch     = errors.New("authengine: microsoft sign-in came from a different tenant")
	errDryRun             = errors.New("authengine: test sign-in stopped before creating anything")
)

// gatedProvider wraps a library provider to keep the in-house sign-in rules:
// verified email required (Microsoft excepted, as before), the provider's
// allowed email domain for new accounts, and Graph ids for Microsoft.
type gatedProvider struct {
	inner    theauth.Provider
	rt       *oauthRuntime
	settings store.OAuthProviderSettings
}

type gatedNonceProvider struct {
	*gatedProvider
	np theauth.NonceProvider
}

func (rt *oauthRuntime) gate(inner theauth.Provider, s store.OAuthProviderSettings) theauth.Provider {
	g := &gatedProvider{inner: inner, rt: rt, settings: s}
	if np, ok := inner.(theauth.NonceProvider); ok {
		return &gatedNonceProvider{gatedProvider: g, np: np}
	}
	return g
}

func (g *gatedProvider) Name() string { return g.inner.Name() }

func (g *gatedProvider) AuthURL(state, challenge, redirectURI string, scopes []string) string {
	return g.inner.AuthURL(state, challenge, redirectURI, scopes)
}

func (g *gatedProvider) ExchangeCode(ctx context.Context, code, verifier, redirectURI string) (*theauth.ProviderToken, error) {
	flowFrom(ctx).markExchange()
	tok, err := g.inner.ExchangeCode(ctx, code, verifier, redirectURI)
	if err != nil {
		flowFrom(ctx).fail(OAuthErrExchangeFailed)
		return nil, err
	}
	return tok, nil
}

func (g *gatedNonceProvider) AuthURLWithNonce(state, challenge, nonce, redirectURI string, scopes []string) string {
	return g.np.AuthURLWithNonce(state, challenge, nonce, redirectURI, scopes)
}

func (g *gatedNonceProvider) ExchangeCodeWithNonce(ctx context.Context, code, verifier, redirectURI, nonce string) (*theauth.ProviderToken, error) {
	flowFrom(ctx).markExchange()
	tok, err := g.np.ExchangeCodeWithNonce(ctx, code, verifier, redirectURI, nonce)
	if err != nil {
		flowFrom(ctx).fail(OAuthErrExchangeFailed)
		return nil, err
	}
	if tok != nil {
		flowFrom(ctx).setVerifiedIDToken(tok.IDToken)
	}
	return tok, nil
}

func (g *gatedProvider) UserInfo(ctx context.Context, tok *theauth.ProviderToken) (*theauth.ProviderUser, error) {
	flow := flowFrom(ctx)
	info, err := g.fetch(ctx, tok)
	if err != nil {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, err
	}
	facts := OAuthIdentityFacts{Provider: g.Name(), ProviderUserID: info.ID, Email: info.Email,
		EmailVerified: info.EmailVerified || emailVerifiedByProvider(g.Name())}
	if idToken := flow.verifiedIDToken(); idToken != "" {
		facts.Claims = ReadIDTokenClaims(idToken, g.settings.SSOPolicy.GroupsClaim)
	}
	flow.record(facts)
	if flow.isDryRun() {
		flow.fail(OAuthErrDryRunComplete)
		return nil, errDryRun
	}
	if info.ID == "" || info.Email == "" {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, errUserInfoIncomplete
	}
	if g.Name() == store.OAuthProviderMicrosoft && g.settings.TenantID != "" &&
		!strings.EqualFold(accessTokenTenant(tok), g.settings.TenantID) {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, errTenantMismatch
	}
	if !facts.EmailVerified {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, fmt.Errorf("authengine: %s email is not verified", g.Name())
	}
	if !EmailDomainAllowed(info.Email, g.settings.SSOPolicy.AllowedDomains) {
		flow.fail(OAuthErrDomainNotAllowed)
		return nil, errDomainNotAllowed
	}
	if err := g.rt.checkSignup(ctx, g.Name(), g.settings, info); err != nil {
		switch {
		case errors.Is(err, errEmailInUse):
			flow.fail(OAuthErrEmailInUse)
		case errors.Is(err, errDomainNotAllowed):
			flow.fail(OAuthErrDomainNotAllowed)
		case errors.Is(err, errNotProvisioned):
			flow.fail(OAuthErrNotProvisioned)
		}
		return nil, err
	}
	flow.identify(g.Name(), info.ID)
	return info, nil
}

// emailVerifiedByProvider lists providers whose userinfo carries no verified
// flag but only returns confirmed addresses (Microsoft Graph, Facebook Graph,
// and Linear, Figma and Codeberg, which confirm an address before it can be
// the profile email).
func emailVerifiedByProvider(name string) bool {
	switch name {
	case store.OAuthProviderMicrosoft, store.OAuthProviderFacebook,
		store.OAuthProviderLinear, store.OAuthProviderFigma, store.OAuthProviderCodeberg:
		return true
	}
	return false
}

func (g *gatedProvider) fetch(ctx context.Context, tok *theauth.ProviderToken) (*theauth.ProviderUser, error) {
	if g.Name() == store.OAuthProviderMicrosoft {
		return g.rt.graphUserInfo(ctx, tok)
	}
	return g.inner.UserInfo(ctx, tok)
}

// graphUserInfo reads Microsoft Graph /me so the subject is the Graph object id the
// in-house flow stored, not the pairwise OIDC sub the library provider would return.
func (rt *oauthRuntime) graphUserInfo(ctx context.Context, tok *theauth.ProviderToken) (*theauth.ProviderUser, error) {
	if tok == nil || tok.AccessToken == "" {
		return nil, errors.New("authengine: microsoft token missing")
	}
	endpoint := rt.wiring.Endpoints.MicrosoftGraphMe
	if endpoint == "" {
		endpoint = defaultGraphMeURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("authengine: microsoft graph request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := (&providerResolver{rt: rt}).httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("authengine: microsoft graph: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authengine: microsoft graph: status %d", resp.StatusCode)
	}
	var body struct {
		ID                string `json:"id"`
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
		DisplayName       string `json:"displayName"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUserInfoBody)).Decode(&body); err != nil {
		return nil, fmt.Errorf("authengine: decode microsoft graph: %w", err)
	}
	email := body.Mail
	if email == "" {
		email = body.UserPrincipalName
	}
	name := body.DisplayName
	if name == "" {
		name = email
	}
	return &theauth.ProviderUser{ID: body.ID, Email: email, Name: name}, nil
}

// accessTokenTenant reads the tid claim of the token the token endpoint just
// returned over TLS; an opaque or foreign token yields "" and is refused.
func accessTokenTenant(tok *theauth.ProviderToken) string {
	if tok == nil {
		return ""
	}
	payload := idTokenPayload(tok.AccessToken)
	if payload == nil {
		return ""
	}
	var c struct {
		TID string `json:"tid"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return ""
	}
	return c.TID
}

// ValidEntraTenantID reports whether s looks like a directory id (a GUID).
func ValidEntraTenantID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case !strings.ContainsRune("0123456789abcdefABCDEF", r):
			return false
		}
	}
	return true
}

// checkSignup mirrors the in-house order: a linked identity always signs in; an
// email held by another account is refused (the library links only verified
// emails); only a brand new account is held to the provider's allowed domain.
func (rt *oauthRuntime) checkSignup(ctx context.Context, provider string, s store.OAuthProviderSettings, info *theauth.ProviderUser) error {
	linked, err := rt.exists(ctx, `SELECT 1 FROM theauth_oauth_accounts WHERE provider = ? AND provider_user_id = ?`, provider, info.ID)
	if err != nil || linked {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	var legacyID, engineID sql.NullString
	err = rt.db.QueryRowContext(ctx, `
		SELECT u.id, m.engine_id FROM users u
		LEFT JOIN authengine_user_map m ON m.legacy_id = u.id
		WHERE lower(u.email) = ?`, email).Scan(&legacyID, &engineID)
	switch {
	case err == nil:
		if !engineID.Valid || !info.EmailVerified {
			return errEmailInUse
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("authengine: look up user by email: %w", err)
	}
	held, err := rt.exists(ctx, `SELECT 1 FROM theauth_users WHERE email = ?`, email)
	if err != nil {
		return err
	}
	if held {
		if !info.EmailVerified {
			return errEmailInUse
		}
		return nil
	}
	// On the shared tenant the mail attribute is controlled by whoever owns the
	// signing-in directory, so it cannot back a new account.
	if provider == store.OAuthProviderMicrosoft && s.TenantID == "" {
		return errTenantRequired
	}
	if !s.SSOPolicy.JITProvisioning {
		return errNotProvisioned
	}
	if s.AllowedEmailDomain != "" && !emailInDomain(email, s.AllowedEmailDomain) {
		return errDomainNotAllowed
	}
	return nil
}

func (rt *oauthRuntime) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var one int
	err := rt.db.QueryRowContext(ctx, query, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("authengine: oauth signup check: %w", err)
	}
	return true, nil
}

// EmailDomainAllowed reports whether email is in one of domains; an empty list allows all.
func EmailDomainAllowed(email string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	host := email[at+1:]
	for _, d := range domains {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(d), "@"), host) {
			return true
		}
	}
	return false
}

func emailInDomain(email, domain string) bool {
	at := strings.LastIndex(email, "@")
	return at >= 0 && strings.EqualFold(email[at+1:], domain)
}
