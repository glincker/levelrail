package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// ExperimentalDisabledCode is the error code returned for a gated route.
const ExperimentalDisabledCode = "experimental_feature_disabled"

type experimentalError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Feature string `json:"feature"`
}

var databaseEnvironmentPath = regexp.MustCompile(`^/api/v1/databases/[^/]+/environment$`)

var gitRemoteAppPath = regexp.MustCompile(`^/api/v1/apps/[^/]+/git-remote$`)

var lbAppPath = regexp.MustCompile(`^/api/v1/apps/[^/]+/loadbalancer(/|$)`)

// featureForPath maps a request path to the experimental feature guarding it.
func featureForPath(p string) (experimental.Feature, bool) {
	switch {
	case gitRemoteAppPath.MatchString(p):
		return experimental.GitPush, true
	case p == "/api/v1/ai/sessions" || strings.HasPrefix(p, "/api/v1/ai/sessions/"),
		p == "/api/v1/settings/ai-assistant", strings.HasPrefix(p, "/api/v1/settings/ai-assistant/"),
		p == "/api/v1/ai/usage":
		return experimental.AIChat, true
	case p == "/api/v1/models" || strings.HasPrefix(p, "/api/v1/models/"), p == "/api/v1/model-endpoints/test":
		return experimental.AIModels, true
	case p == "/api/v1/loadbalancers", lbAppPath.MatchString(p):
		return experimental.LoadBalancer, true
	case p == "/api/v1/apply" || strings.HasPrefix(p, "/api/v1/apply/"), p == "/api/v1/export", p == "/api/v1/stacks" || strings.HasPrefix(p, "/api/v1/stacks/"):
		return experimental.IaC, true
	case p == "/api/v1/settings/cloudflare-tunnel":
		return experimental.CloudflareTunnel, true
	case p == "/api/v1/environments", databaseEnvironmentPath.MatchString(p):
		return experimental.GlobalEnvironments, true
	case strings.HasPrefix(p, "/api/v1/resource-advisor/"):
		return experimental.ResourceAdvisor, true
	case p == "/api/v1/plugins" || strings.HasPrefix(p, "/api/v1/plugins/"), p == "/api/v1/plugin-keys" || strings.HasPrefix(p, "/api/v1/plugin-keys/"), p == "/api/v1/plugin-panels", p == "/api/v1/plugin-marketplace" || strings.HasPrefix(p, "/api/v1/plugin-marketplace/"):
		return experimental.Plugins, true
	case strings.HasPrefix(p, mobileRoutePrefix):
		return experimental.MobileAPI, true
	case strings.HasPrefix(p, "/api/v1/auth/saml/"), strings.HasPrefix(p, "/api/v1/sso/"), strings.HasPrefix(p, "/api/v1/scim/"):
		return experimental.SAMLSCIM, true
	}
	return "", false
}

// environmentListForRoles lets the guest grants editor read the environment
// list when only access-roles is on. Creating environments stays gated.
func environmentListForRoles(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/api/v1/environments" && experimental.Enabled(experimental.AccessRoles)
}

// featureOn is experimental.Enabled plus per-feature env kill switches.
func featureOn(f experimental.Feature) bool {
	if f == experimental.MobileAPI {
		return mobileAPIEnabled()
	}
	return experimental.Enabled(f)
}

// enabledFeatures is experimental.EnabledList with kill switches applied.
func enabledFeatures() []experimental.Feature {
	out := []experimental.Feature{}
	for _, f := range experimental.EnabledList() {
		if featureOn(f) {
			out = append(out, f)
		}
	}
	return out
}

// experimentalGateMiddleware answers 404 for routes of features that are off.
func experimentalGateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, gated := featureForPath(r.URL.Path); gated && !featureOn(f) && !environmentListForRoles(r) {
			writeJSON(w, http.StatusNotFound, experimentalError{
				Error:   experimental.DisabledMessage(f),
				Code:    ExperimentalDisabledCode,
				Feature: string(f),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
