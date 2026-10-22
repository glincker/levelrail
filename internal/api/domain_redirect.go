package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// DomainRedirectStore is the core store surface for domain redirects.
// It has no secrets dependency, since neither the target URL nor the status code is credential material.
type DomainRedirectStore interface {
	GetDomainRedirect(ctx context.Context, domain string) (store.DomainRedirect, bool, error)
	SetDomainRedirect(ctx context.Context, domain, targetURL string, statusCode int) error
	DeleteDomainRedirect(ctx context.Context, domain string) error
	// ListDomainRedirects backs GET /api/v1/domains's has_redirect flag:
	// one bulk read instead of a per-domain GetDomainRedirect call,
	// avoiding an N+1 query over the domain list.
	ListDomainRedirects(ctx context.Context) ([]store.DomainRedirect, error)
}

// Enabled is derived from whether a row exists, not stored as its own column.
type domainRedirectResource struct {
	Domain     string `json:"domain"`
	Enabled    bool   `json:"enabled"`
	TargetURL  string `json:"target_url,omitempty"`
	StatusCode int    `json:"status_code"`
}

func toDomainRedirectResource(domain string, row store.DomainRedirect, found bool) domainRedirectResource {
	statusCode := row.StatusCode
	if statusCode == 0 {
		statusCode = store.DomainRedirectPermanent
	}
	return domainRedirectResource{
		Domain:     domain,
		Enabled:    found,
		TargetURL:  row.TargetURL,
		StatusCode: statusCode,
	}
}

// A zero StatusCode defaults to store.DomainRedirectPermanent (301), matching Caddy's default.
type setDomainRedirectRequest struct {
	TargetURL  string `json:"target_url"`
	StatusCode int    `json:"status_code,omitempty"`
}

// Requires an absolute http(s) URL with a host, since a browser must be able to follow the target.
func validateRedirectTargetURL(raw string) error {
	if raw == "" {
		return errors.New("target_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("target_url must be an absolute http(s) URL, e.g. https://example.com")
	}
	return nil
}

// AbilityRead, the same visibility tier as GET .../maintenance.
func (rt *Router) handleGetDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	row, found, err := rt.domainRedirect.GetDomainRedirect(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain redirect failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, row, found))
}

// AbilityDeploy, not AbilityRoot: this changes routing, not a credential.
func (rt *Router) handleSetDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainRedirectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateRedirectTargetURL(req.TargetURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	statusCode := req.StatusCode
	if statusCode == 0 {
		statusCode = store.DomainRedirectPermanent
	}
	if !trafficpolicy.AliasStatuses[statusCode] {
		writeError(w, http.StatusBadRequest, "status_code must be 301, 302, 307 or 308")
		return
	}
	if rows, err := rt.hostRedirects(r.Context()); err == nil {
		if err := trafficpolicy.CheckRedirectLoop(rows, domain, trafficpolicy.TargetHost(req.TargetURL)); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}

	if err := rt.domainRedirect.SetDomainRedirect(r.Context(), domain, req.TargetURL, statusCode); err != nil {
		rt.internalError(w, "api: set domain redirect failed", err, slog.String("domain", domain))
		return
	}

	row, found, err := rt.domainRedirect.GetDomainRedirect(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain redirect failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, row, found))
}

// Idempotent: clearing a domain with no redirect is not an error.
func (rt *Router) handleClearDomainRedirect(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainRedirect.DeleteDomainRedirect(r.Context(), domain); err != nil {
		rt.internalError(w, "api: clear domain redirect failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainRedirectResource(domain, store.DomainRedirect{}, false))
}
