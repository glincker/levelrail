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
	return tok, nil
}

func (g *gatedProvider) UserInfo(ctx context.Context, tok *theauth.ProviderToken) (*theauth.ProviderUser, error) {
	flow := flowFrom(ctx)
	info, err := g.fetch(ctx, tok)
	if err != nil {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, err
	}
	if info.ID == "" || info.Email == "" {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, errUserInfoIncomplete
	}
	if !info.EmailVerified && g.Name() != store.OAuthProviderMicrosoft {
		flow.fail(OAuthErrUserInfoFailed)
		return nil, fmt.Errorf("authengine: %s email is not verified", g.Name())
	}
	if err := g.rt.checkSignup(ctx, g.Name(), g.settings, info); err != nil {
		switch {
		case errors.Is(err, errEmailInUse):
			flow.fail(OAuthErrEmailInUse)
		case errors.Is(err, errDomainNotAllowed):
			flow.fail(OAuthErrDomainNotAllowed)
		}
		return nil, err
	}
	flow.identify(g.Name(), info.ID)
	return info, nil
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

func emailInDomain(email, domain string) bool {
	at := strings.LastIndex(email, "@")
	return at >= 0 && strings.EqualFold(email[at+1:], domain)
}
