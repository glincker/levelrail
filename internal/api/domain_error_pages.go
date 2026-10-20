package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainErrorPagesStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/error-pages need: a domain's status-code-to-HTML
// mappings, always set, same "core Store interface" shape as
// DomainRedirectStore above; no secrets dependency either.
type DomainErrorPagesStore interface {
	ListDomainErrorPages(ctx context.Context, domain string) ([]store.DomainErrorPage, error)
	SetDomainErrorPage(ctx context.Context, domain string, statusCode int, body string) error
	DeleteDomainErrorPage(ctx context.Context, domain string, statusCode int) error
	DeleteDomainErrorPages(ctx context.Context, domain string) error
}

// domainErrorPagesAllowedStatusCodes is the fixed, small set of status
// codes this feature covers: the common cases an operator actually
// wants a custom page for, not a fully generic arbitrary-status-code
// system.
var domainErrorPagesAllowedStatusCodes = map[int]bool{
	store.DomainErrorPageNotFound:           true,
	store.DomainErrorPageServerError:        true,
	store.DomainErrorPageBadGateway:         true,
	store.DomainErrorPageServiceUnavailable: true,
}

// domainErrorPageEntry is one status-code-to-body mapping in the wire
// shape below.
type domainErrorPageEntry struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

// domainErrorPagesResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/error-pages: every custom error page currently
// configured for domain. Pages is an empty (never nil) list when none
// are configured, not a separate "enabled" flag: this resource is a
// collection, unlike domainRedirectResource's single value.
type domainErrorPagesResource struct {
	Domain string                 `json:"domain"`
	Pages  []domainErrorPageEntry `json:"pages"`
}

func toDomainErrorPagesResource(domain string, rows []store.DomainErrorPage) domainErrorPagesResource {
	pages := make([]domainErrorPageEntry, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, domainErrorPageEntry{StatusCode: row.StatusCode, Body: row.Body})
	}
	return domainErrorPagesResource{Domain: domain, Pages: pages}
}

// setDomainErrorPageRequest is PUT .../error-pages's request body: one
// status-code-to-body upsert at a time, matching the CLI's own
// one-code-per-call "set" verb.
type setDomainErrorPageRequest struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

// handleGetDomainErrorPages handles GET
// /api/v1/apps/{name}/domains/{domain}/error-pages: every custom error
// page currently configured for domain. AbilityRead, the same
// passive-visibility tier GET .../domains/{domain}/redirect already
// uses.
func (rt *Router) handleGetDomainErrorPages(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	rows, err := rt.domainErrorPages.ListDomainErrorPages(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: list domain error pages failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainErrorPagesResource(domain, rows))
}

// handleSetDomainErrorPage handles PUT
// /api/v1/apps/{name}/domains/{domain}/error-pages: upserts one
// status-code-to-body mapping for domain, enforced by Caddy on the next
// ingress reconcile pass. AbilityDeploy, matching PUT .../redirect: this
// changes an app's runtime routing behavior, not a credential.
func (rt *Router) handleSetDomainErrorPage(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainErrorPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !domainErrorPagesAllowedStatusCodes[req.StatusCode] {
		writeError(w, http.StatusBadRequest, "status_code must be one of 404, 500, 502, 503")
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	if err := rt.domainErrorPages.SetDomainErrorPage(r.Context(), domain, req.StatusCode, req.Body); err != nil {
		rt.logger.Error("api: set domain error page failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := rt.domainErrorPages.ListDomainErrorPages(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: list domain error pages failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainErrorPagesResource(domain, rows))
}

// handleClearDomainErrorPages handles DELETE
// /api/v1/apps/{name}/domains/{domain}/error-pages: removes one mapping
// when ?status_code=NNN is given, or every mapping for domain when it's
// omitted, taking effect on the next ingress reconcile pass.
// AbilityDeploy. Idempotent: clearing a mapping or a domain with none
// configured is not an error.
func (rt *Router) handleClearDomainErrorPages(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if raw := r.URL.Query().Get("status_code"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "status_code must be an integer")
			return
		}
		if err := rt.domainErrorPages.DeleteDomainErrorPage(r.Context(), domain, code); err != nil {
			rt.logger.Error("api: clear domain error page failed", slog.String("error", err.Error()), slog.String("domain", domain))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	} else if err := rt.domainErrorPages.DeleteDomainErrorPages(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain error pages failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := rt.domainErrorPages.ListDomainErrorPages(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: list domain error pages failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainErrorPagesResource(domain, rows))
}
