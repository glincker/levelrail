package authengine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// EnvOAuthAllowedHosts adds comma separated hosts (or host:port) allowed in an OAuth redirect URI.
const EnvOAuthAllowedHosts = "APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS"

// EnvDashboardURL pins the public dashboard URL, taking precedence over the stored setting when the setting is empty.
const EnvDashboardURL = "APP_DASHBOARD_URL"

// OAuthCallbackPath is the callback route registered at identity providers.
func OAuthCallbackPath(provider string) string {
	return "/api/v1/auth/oauth/" + provider + "/callback"
}

// ErrOAuthRedirectUnavailable means no trustworthy base URL exists for the OAuth redirect URI.
var ErrOAuthRedirectUnavailable = errors.New("authengine: oauth redirect: set the dashboard URL (or APP_DASHBOARD_URL) before signing in with a provider")

// OAuthRedirectResolver builds the provider redirect URI. The dashboard URL
// wins; without one the Host and X-Forwarded-Proto headers are used only
// from a trusted peer, since otherwise a caller picks the URI sent to the IdP.
type OAuthRedirectResolver struct {
	DashboardURL func(ctx context.Context) string
	TrustedPeer  func(r *http.Request) bool
}

// Resolve returns the redirect URI for provider, or ErrOAuthRedirectUnavailable.
func (o OAuthRedirectResolver) Resolve(r *http.Request, provider string) (string, error) {
	if r == nil {
		return "", errors.New("authengine: oauth redirect: no request")
	}
	if base := o.dashboardBase(r.Context()); base != "" {
		return base + OAuthCallbackPath(provider), nil
	}
	trusted := o.TrustedPeer
	if trusted == nil {
		trusted = loopbackPeer
	}
	if r.Host == "" || !trusted(r) {
		return "", ErrOAuthRedirectUnavailable
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); proto == "http" || proto == "https" {
		scheme = proto
	}
	return scheme + "://" + r.Host + OAuthCallbackPath(provider), nil
}

func loopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func (o OAuthRedirectResolver) dashboardBase(ctx context.Context) string {
	raw := ""
	if o.DashboardURL != nil {
		raw = o.DashboardURL(ctx)
	}
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv(EnvDashboardURL)
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return raw
}

// oauthAllowedHosts limits redirect hosts to the dashboard and base URL hosts plus
// the env extras. Empty (no dashboard URL set yet) leaves the host unrestricted.
func oauthAllowedHosts(cfg Config) []string {
	var out []string
	add := func(raw string) {
		if raw = strings.TrimSpace(raw); raw == "" {
			return
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			out = append(out, u.Host)
			return
		}
		out = append(out, raw)
	}
	for _, h := range strings.Split(os.Getenv(EnvOAuthAllowedHosts), ",") {
		add(h)
	}
	if cfg.MFA.DashboardURL == "" && len(out) == 0 {
		return nil
	}
	add(cfg.MFA.DashboardURL)
	add(cfg.BaseURL)
	return out
}
