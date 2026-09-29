package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/oidc"
)

// defaultOIDCJWKSRatePerMinute bounds the unauthenticated JWKS endpoint:
// generous enough for a cloud provider's own key-rotation polling
// (AWS/GCP/Vault all cache the document, typically for hours) while still
// capping a scraper. Overridable via APP_OIDC_JWKS_RATE_LIMIT_PER_MINUTE.
const defaultOIDCJWKSRatePerMinute = 60

// OIDCJWKSProvider serves the public half of the signing key pipeline
// jobs' OIDC tokens are signed with. *oidc.Manager satisfies it.
type OIDCJWKSProvider interface {
	JWKS(ctx context.Context) (oidc.JWKS, error)
}

// SetOIDCManager wires the OIDC signing key manager: it serves the
// public GET /.well-known/jwks.json document and the authenticated
// summary GET /api/v1/pipelines/oidc reads for the dashboard. Minting
// the tokens themselves is wired separately, into
// internal/pipeline.Config.OIDCIssuer, since that lives on the pipeline
// engine, not this router.
func (rt *Router) SetOIDCManager(m OIDCJWKSProvider, issuerURL string, ratePerMinute int) {
	rt.oidcJWKS = m
	rt.oidcIssuerURL = strings.TrimRight(issuerURL, "/")
	if ratePerMinute <= 0 {
		ratePerMinute = defaultOIDCJWKSRatePerMinute
	}
	rt.oidcJWKSLimiter = newAPIRateLimiter(ratePerMinute)
}

// oidcInfoResource is what GET /api/v1/pipelines/oidc returns: enough for
// the dashboard to show a job's `oidc: {audience: ...}` config is wired to
// something real, with the URLs an operator copies into an AWS IAM OIDC
// provider, a GCP workload identity pool, or a Vault JWT auth mount.
type oidcInfoResource struct {
	Configured bool   `json:"configured"`
	IssuerURL  string `json:"issuer_url,omitempty"`
	JWKSURL    string `json:"jwks_url,omitempty"`
}

// handleGetPipelineOIDCInfo handles GET /api/v1/pipelines/oidc.
func (rt *Router) handleGetPipelineOIDCInfo(w http.ResponseWriter, _ *http.Request) {
	out := oidcInfoResource{Configured: rt.oidcJWKS != nil && rt.oidcIssuerURL != ""}
	if out.Configured {
		out.IssuerURL = rt.oidcIssuerURL
		out.JWKSURL = rt.oidcIssuerURL + "/.well-known/jwks.json"
	}
	writeJSON(w, http.StatusOK, out)
}

// handleOIDCJWKS handles GET /.well-known/jwks.json: unauthenticated by
// design, the same way every OIDC issuer's JWKS document is meant to be
// fetched by a verifier (AWS IAM, GCP workload identity federation,
// Vault) that has no credential of its own yet.
func (rt *Router) handleOIDCJWKS(w http.ResponseWriter, r *http.Request) {
	if rt.oidcJWKS == nil {
		writeError(w, http.StatusNotFound, "OIDC is not configured on this control plane")
		return
	}
	if rt.oidcJWKSLimiter != nil {
		if ok, retryAfter := rt.oidcJWKSLimiter.allow(clientIP(r)); !ok {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	jwks, err := rt.oidcJWKS.JWKS(r.Context())
	if err != nil {
		rt.internalError(w, "oidc: build jwks", err)
		return
	}
	writeJSON(w, http.StatusOK, jwks)
}
