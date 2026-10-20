package api

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

const (
	envSSORoutePerMinute     = "APP_SSO_ROUTE_RATE_PER_MINUTE"
	defaultSSORoutePerMinute = 60
	ssoDomainQuery           = "sso_domain"
	maxSSODomainLen          = 253
)

// ssoRoute is the connection a domain signs in through.
type ssoRoute struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	StartURL string `json:"start_url"`
}

func newSSORouteLimiter() *apiRateLimiter {
	return newWindowRateLimiter(envInt(envSSORoutePerMinute, defaultSSORoutePerMinute), time.Minute)
}

// routeSSODomain maps the ?sso_domain= of a login-options request to the one
// enabled connection that claims it. The page sends only the part after the @,
// never an address, so the lookup cannot say anything about a person. A domain
// with no match, with two matches (ambiguous, so refused), or a malformed or
// rate limited request all answer nil.
func (rt *Router) routeSSODomain(r *http.Request) *ssoRoute {
	raw := r.URL.Query().Get(ssoDomainQuery)
	if raw == "" || len(raw) > maxSSODomainLen || !featureOn(experimental.SAMLSCIM) {
		return nil
	}
	domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
	if !samlDomainRe.MatchString(domain) {
		return nil
	}
	if rt.ssoRouteLimiter != nil {
		if ok, _ := rt.ssoRouteLimiter.allow(clientIP(r)); !ok {
			return nil
		}
	}
	var matches []ssoRoute
	ctx := r.Context()
	if rt.sso != nil && rt.authEngine != nil && rt.authEngine.SAMLEnabled() {
		if conns, err := rt.sso.ListSSOConnections(ctx); err == nil {
			for _, c := range conns {
				if samlUsable(c) && slices.Contains(c.AllowedDomains, domain) {
					matches = append(matches, ssoRoute{Kind: ssoKindSAML, ID: c.ID, Name: c.Name,
						StartURL: samlPathPrefix + url.PathEscape(c.ID) + "/login"})
				}
			}
		}
	}
	if rt.oauthSettings != nil && rt.authLibOAuthActive() {
		if all, err := rt.oauthSettings.ListOAuthProviderSettings(ctx); err == nil {
			for _, s := range all {
				if s.Enabled && slices.Contains(s.SSOPolicy.AllowedDomains, domain) {
					matches = append(matches, ssoRoute{Kind: ssoKindOAuth, ID: s.Provider, Name: oauthDisplayName(s),
						StartURL: oauthCallbackAuditPath + s.Provider + "/start"})
				}
			}
		}
	}
	if len(matches) != 1 {
		return nil
	}
	return &matches[0]
}
