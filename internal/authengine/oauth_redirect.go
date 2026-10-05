package authengine

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// EnvOAuthAllowedHosts adds comma separated hosts (or host:port) allowed in an OAuth redirect URI.
const EnvOAuthAllowedHosts = "APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS"

// OAuthCallbackPath is the callback route registered at identity providers.
func OAuthCallbackPath(provider string) string {
	return "/api/v1/auth/oauth/" + provider + "/callback"
}

// oauthRedirectURI builds the provider redirect URI from the request, the same
// way the built-in flow does, so callback URLs registered at providers stay valid.
func oauthRedirectURI(r *http.Request, provider string) (string, error) {
	if r == nil || r.Host == "" {
		return "", errors.New("authengine: oauth redirect: request has no host")
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + OAuthCallbackPath(provider), nil
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
