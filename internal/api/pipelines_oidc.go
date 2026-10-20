package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

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

// OIDCKeyRotator rotates the pipeline OIDC signing key. *oidc.Manager
// satisfies it. A separate interface from OIDCJWKSProvider, not a wider
// one: a fake in a JWKS-only test has no reason to implement rotation too.
type OIDCKeyRotator interface {
	RotateKey(ctx context.Context, retireAfter time.Duration) (oidc.RotationResult, error)
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

// SetOIDCRotator wires POST /api/v1/pipelines/oidc/rotate-key. Optional:
// nil (the default) means the endpoint returns 501, the same
// not-configured shape SetOIDCManager's own JWKS route uses.
func (rt *Router) SetOIDCRotator(r OIDCKeyRotator) {
	rt.oidcRotator = r
}

// oidcInfoResource is what GET /api/v1/pipelines/oidc returns: enough for
// the dashboard to show a job's `oidc: {audience: ...}` config is wired to
// something real, with the URLs an operator copies into an AWS IAM OIDC
// provider, a GCP workload identity pool, or a Vault JWT auth mount.
type oidcInfoResource struct {
	Configured        bool   `json:"configured"`
	IssuerURL         string `json:"issuer_url,omitempty"`
	JWKSURL           string `json:"jwks_url,omitempty"`
	RotationSupported bool   `json:"rotation_supported"`
}

// handleGetPipelineOIDCInfo handles GET /api/v1/pipelines/oidc.
func (rt *Router) handleGetPipelineOIDCInfo(w http.ResponseWriter, _ *http.Request) {
	out := oidcInfoResource{Configured: rt.oidcJWKS != nil && rt.oidcIssuerURL != "", RotationSupported: rt.oidcRotator != nil}
	if out.Configured {
		out.IssuerURL = rt.oidcIssuerURL
		out.JWKSURL = rt.oidcIssuerURL + "/.well-known/jwks.json"
	}
	writeJSON(w, http.StatusOK, out)
}

type rotatePipelineOIDCKeyRequest struct {
	// RetireAfter is a Go duration string (e.g. "1h") the previous
	// signing key stays published in the JWKS for. Empty uses
	// oidc.DefaultKeyRetireGrace.
	RetireAfter string `json:"retire_after,omitempty"`
}

type rotatePipelineOIDCKeyResponse struct {
	OldKID        string    `json:"old_kid"`
	NewKID        string    `json:"new_kid"`
	RetireAt      time.Time `json:"retire_at"`
	RetiringCount int       `json:"retiring_count"`
}

// handleRotatePipelineOIDCKey handles POST /api/v1/pipelines/oidc/rotate-key:
// generates a fresh signing key and makes it active immediately. The
// previous key keeps verifying, published in the JWKS, until RetireAt:
// AWS, GCP, and Vault all cache the JWKS document for a while, so an
// immediate hard cutover would fail in-flight verifications for no
// reason. See docs/pipelines-oidc.md.
func (rt *Router) handleRotatePipelineOIDCKey(w http.ResponseWriter, r *http.Request) {
	if rt.oidcRotator == nil {
		writeError(w, http.StatusNotImplemented, "OIDC key rotation is not configured on this control plane")
		return
	}
	var req rotatePipelineOIDCKeyRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	var retireAfter time.Duration
	if strings.TrimSpace(req.RetireAfter) != "" {
		d, err := time.ParseDuration(req.RetireAfter)
		if err != nil {
			writeError(w, http.StatusBadRequest, "retire_after: "+err.Error())
			return
		}
		retireAfter = d
	}

	res, err := rt.oidcRotator.RotateKey(r.Context(), retireAfter)
	if err != nil {
		rt.logger.Error("api: rotate pipeline oidc key failed", slog.String("error", err.Error()))
		rt.internalError(w, "oidc: rotate signing key", err)
		return
	}
	writeJSON(w, http.StatusOK, rotatePipelineOIDCKeyResponse{
		OldKID: res.OldKID, NewKID: res.NewKID, RetireAt: res.RetireAt, RetiringCount: res.RetiringCount,
	})
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
