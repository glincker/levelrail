package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
	bitbucketprov "github.com/glincker/theauth-go/v2/provider/bitbucket"
	discordprov "github.com/glincker/theauth-go/v2/provider/discord"
	facebookprov "github.com/glincker/theauth-go/v2/provider/facebook"
	genericprov "github.com/glincker/theauth-go/v2/provider/generic"
	githubprov "github.com/glincker/theauth-go/v2/provider/github"
	gitlabprov "github.com/glincker/theauth-go/v2/provider/gitlab"
	googleprov "github.com/glincker/theauth-go/v2/provider/google"
	linkedinprov "github.com/glincker/theauth-go/v2/provider/linkedin"
	microsoftprov "github.com/glincker/theauth-go/v2/provider/microsoft"
	oidcprov "github.com/glincker/theauth-go/v2/provider/oidc"
	slackprov "github.com/glincker/theauth-go/v2/provider/slack"
	twitchprov "github.com/glincker/theauth-go/v2/provider/twitch"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// defaultOIDCClient refuses internal destinations (discovery and token calls go to operator-supplied issuers).
var defaultOIDCClient = sync.OnceValue(func() *http.Client {
	c := netguard.NewClient()
	c.Timeout = oidcDiscoveryTimeout
	return c
})

const oidcDiscoveryTimeout = 10 * time.Second

// Scope sets match what the in-house flow requests, so consent screens do not change.
var (
	googleScopes    = []string{"openid", "email", "profile"}
	githubScopes    = []string{"read:user", "user:email"}
	microsoftScopes = []string{"openid", "email", "profile", "https://graph.microsoft.com/User.Read"}
	oidcScopes      = []string{"openid", "email", "profile"}
	gitlabScopes    = []string{"openid", "email", "profile"}
	bitbucketScopes = []string{"account", "email"}
	discordScopes   = []string{"identify", "email"}
	slackScopes     = []string{"openid", "email", "profile"}
	linkedinScopes  = []string{"openid", "email", "profile"}
	twitchScopes    = []string{"openid", "user:read:email"}
	facebookScopes  = []string{"email", "public_profile"}
)

// providerResolver builds library providers from the dashboard settings on
// every cache miss, so edits apply without a restart.
type providerResolver struct{ rt *oauthRuntime }

func newProviderResolver(rt *oauthRuntime) *providerResolver { return &providerResolver{rt: rt} }

func validOAuthProvider(name string) bool {
	return store.IsOAuthProvider(name)
}

// Resolve returns the enabled provider for name. A settings or secret read
// failure is an error, never "not found", so the flow fails closed.
func (r *providerResolver) Resolve(ctx context.Context, name string) (theauth.Provider, bool, error) {
	if !validOAuthProvider(name) {
		return nil, false, nil
	}
	w := r.rt.wiring
	s, err := w.Settings.GetOAuthProviderSettings(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("authengine: load oauth settings for %s: %w", name, err)
	}
	if !s.Enabled {
		return nil, false, nil
	}
	secret, err := w.Secrets.Resolve(ctx, store.OAuthProviderSecretsKey(name), store.OAuthProviderSecretEnvKey)
	if err != nil {
		return nil, false, fmt.Errorf("authengine: resolve client secret for %s: %w", name, err)
	}
	if s.ClientID == "" || secret == "" {
		return nil, false, fmt.Errorf("authengine: provider %s is enabled without client credentials", name)
	}
	inner, err := r.build(ctx, s, secret)
	if err != nil {
		return nil, false, err
	}
	return r.rt.gate(inner, s), true, nil
}

func (r *providerResolver) httpClient() *http.Client {
	if r.rt.wiring.HTTP != nil {
		return r.rt.wiring.HTTP
	}
	return defaultOIDCClient()
}

func (r *providerResolver) build(ctx context.Context, s store.OAuthProviderSettings, secret string) (theauth.Provider, error) {
	ep := r.rt.wiring.Endpoints
	hc := r.httpClient()
	switch s.Provider {
	case store.OAuthProviderGoogle:
		return googleprov.New(googleprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: googleScopes, HTTPClient: hc,
			AuthorizeURL: ep.GoogleAuthorize, TokenURL: ep.GoogleToken, UserInfoURL: ep.GoogleUserInfo,
		}), nil
	case store.OAuthProviderGitHub:
		return githubprov.New(githubprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: githubScopes, HTTPClient: hc,
			AuthorizeURL: ep.GitHubAuthorize, TokenURL: ep.GitHubToken, UserURL: ep.GitHubUser, EmailsURL: ep.GitHubEmails,
		}), nil
	case store.OAuthProviderMicrosoft:
		tenant := s.TenantID
		if tenant == "" {
			tenant = "common"
		}
		return microsoftprov.New(microsoftprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: microsoftScopes, Tenant: tenant, HTTPClient: hc,
			AuthorizeURL: ep.MicrosoftAuthorize, TokenURL: ep.MicrosoftToken,
		}), nil
	case store.OAuthProviderGitLab:
		base, err := gitlabBaseURL(s.IssuerURL, ep.AllowInsecureOIDC)
		if err != nil {
			return nil, err
		}
		return gitlabprov.New(gitlabprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: gitlabScopes, BaseURL: base, HTTPClient: hc,
		}), nil
	case store.OAuthProviderBitbucket:
		return bitbucketprov.New(bitbucketprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: bitbucketScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderDiscord:
		return discordprov.New(discordprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: discordScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderSlack:
		return slackprov.New(slackprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: slackScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderLinkedIn:
		return linkedinprov.New(linkedinprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: linkedinScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderTwitch:
		return twitchprov.New(twitchprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: twitchScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderFacebook:
		return facebookprov.New(facebookprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, Scopes: facebookScopes, HTTPClient: hc,
		}), nil
	case store.OAuthProviderLinear, store.OAuthProviderVercel, store.OAuthProviderHuggingFace,
		store.OAuthProviderFigma, store.OAuthProviderSalesforce, store.OAuthProviderCodeberg,
		store.OAuthProviderDropbox, store.OAuthProviderKakao, store.OAuthProviderPatreon:
		return genericprov.NewByName(s.Provider, genericprov.Config{
			ClientID: s.ClientID, ClientSecret: secret, HTTPClient: hc,
		})
	case store.OAuthProviderCognito:
		region, poolID, err := ParseCognitoPoolID(s.IssuerURL)
		if err != nil {
			return nil, err
		}
		cfg := oidcprov.Cognito(region, poolID, s.ClientID, secret)
		cfg.HTTPClient = hc
		dctx, cancel := context.WithTimeout(ctx, oidcDiscoveryTimeout)
		defer cancel()
		p, err := oidcprov.New(dctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("authengine: oidc discovery for provider %s: %w", s.Provider, err)
		}
		return p, nil
	case store.OAuthProviderOIDC:
		dctx, cancel := context.WithTimeout(ctx, oidcDiscoveryTimeout)
		defer cancel()
		p, err := oidcprov.New(dctx, oidcprov.Config{
			Name: store.OAuthProviderOIDC, Issuer: s.IssuerURL, ClientID: s.ClientID, ClientSecret: secret,
			Scopes: oidcScopes, AllowInsecureHTTP: ep.AllowInsecureOIDC, HTTPClient: hc,
		})
		if err != nil {
			return nil, fmt.Errorf("authengine: oidc discovery for provider %s: %w", s.Provider, err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("authengine: unknown oauth provider %q", s.Provider)
	}
}

// gitlabBaseURL returns the self-hosted GitLab root, or "" for gitlab.com.
// Plain http is refused unless the same insecure flag the OIDC path honours is set.
func gitlabBaseURL(raw string, allowInsecure bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("authengine: invalid gitlab instance url")
	}
	if u.Scheme != "https" && (!allowInsecure || u.Scheme != "http") {
		return "", fmt.Errorf("authengine: gitlab instance url must use https")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

var cognitoPoolIDPattern = regexp.MustCompile(`^([a-z]{2}(?:-[a-z]+)+-[0-9])_[A-Za-z0-9]+$`)

// ParseCognitoPoolID splits a user pool id such as "us-east-1_AbC123" into its
// region and the id itself. The region is the id's own prefix, so one stored
// field carries both.
func ParseCognitoPoolID(raw string) (region, poolID string, err error) {
	id := strings.TrimSpace(raw)
	m := cognitoPoolIDPattern.FindStringSubmatch(id)
	if m == nil {
		return "", "", fmt.Errorf("authengine: invalid cognito user pool id")
	}
	return m[1], id, nil
}
