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

var lbAppPath = regexp.MustCompile(`^/api/v1/apps/[^/]+/loadbalancer(/|$)`)

// featureForPath maps a request path to the experimental feature guarding it.
func featureForPath(p string) (experimental.Feature, bool) {
	switch {
	case p == "/api/v1/ai/sessions" || strings.HasPrefix(p, "/api/v1/ai/sessions/"),
		p == "/api/v1/settings/ai-assistant":
		return experimental.AIChat, true
	case p == "/api/v1/models" || strings.HasPrefix(p, "/api/v1/models/"):
		return experimental.AIModels, true
	case p == "/api/v1/loadbalancers", lbAppPath.MatchString(p):
		return experimental.LoadBalancer, true
	case p == "/api/v1/apply" || strings.HasPrefix(p, "/api/v1/apply/"), p == "/api/v1/export":
		return experimental.IaC, true
	case p == "/api/v1/settings/cloudflare-tunnel":
		return experimental.CloudflareTunnel, true
	}
	return "", false
}

// experimentalGateMiddleware answers 404 for routes of features that are off.
func experimentalGateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, gated := featureForPath(r.URL.Path); gated && !experimental.Enabled(f) {
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
