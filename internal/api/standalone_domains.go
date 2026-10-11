package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	domainKindApp        = "app"
	domainKindStandalone = "standalone"
	standaloneBodyLimit  = 16 << 10

	auditActionStandaloneCreated = "standalone_domain.created"
	auditActionStandaloneUpdated = "standalone_domain.updated"
	auditActionStandaloneDeleted = "standalone_domain.deleted"
)

type standaloneDomainStore interface {
	CreateStandaloneDomain(ctx context.Context, d store.StandaloneDomain) error
	UpdateStandaloneDomain(ctx context.Context, d store.StandaloneDomain) error
	GetStandaloneDomain(ctx context.Context, domain string) (store.StandaloneDomain, error)
	ListStandaloneDomains(ctx context.Context) ([]store.StandaloneDomain, error)
	DeleteStandaloneDomain(ctx context.Context, domain string) error
}

func (rt *Router) standaloneStore() standaloneDomainStore {
	s, _ := rt.apps.(standaloneDomainStore)
	return s
}

// standaloneDomainResource is one standalone domain on the wire. Scope is
// the {name} the per-domain endpoints (auth, waf, headers, go-live) take.
type standaloneDomainResource struct {
	Domain             string `json:"domain"`
	Kind               string `json:"kind"`
	Scope              string `json:"scope"`
	Target             string `json:"target"`
	TargetKind         string `json:"target_kind"`
	TargetURL          string `json:"target_url"`
	UpstreamTLS        bool   `json:"upstream_tls"`
	TLSSkipVerify      bool   `json:"tls_skip_verify"`
	TLSServerName      string `json:"tls_server_name,omitempty"`
	HostHeader         string `json:"host_header"`
	WebSocket          bool   `json:"websocket"`
	DialTimeoutSeconds int    `json:"dial_timeout_seconds"`
	ReadTimeoutSeconds int    `json:"read_timeout_seconds"`
	StripPrefix        string `json:"strip_prefix,omitempty"`
	Description        string `json:"description,omitempty"`
	CreatedBy          string `json:"created_by,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

func toStandaloneDomainResource(d store.StandaloneDomain) standaloneDomainResource {
	res := standaloneDomainResource{
		Domain: d.Domain, Kind: domainKindStandalone, Scope: standalone.Scope, Target: d.Target, TargetKind: d.TargetKind,
		UpstreamTLS: d.UpstreamTLS, TLSSkipVerify: d.TLSSkipVerify, TLSServerName: d.TLSServerName,
		HostHeader: d.HostHeader, WebSocket: d.WebSocket, DialTimeoutSeconds: d.DialTimeoutSeconds,
		ReadTimeoutSeconds: d.ReadTimeoutSeconds, StripPrefix: d.StripPrefix, Description: d.Description,
		CreatedBy: d.CreatedBy,
	}
	if t, err := standalone.ParseTarget(d.Target); err == nil {
		res.TargetURL = t.Display(d.UpstreamTLS)
		if res.HostHeader == "" {
			res.HostHeader = t.DefaultHostMode()
		}
	}
	if !d.CreatedAt.IsZero() {
		res.CreatedAt = d.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !d.UpdatedAt.IsZero() {
		res.UpdatedAt = d.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return res
}

// standaloneDomainRequest is POST and PUT's body; PUT leaves nil fields as stored.
type standaloneDomainRequest struct {
	Domain             string  `json:"domain,omitempty"`
	Target             *string `json:"target,omitempty"`
	UpstreamTLS        *bool   `json:"upstream_tls,omitempty"`
	TLSSkipVerify      *bool   `json:"tls_skip_verify,omitempty"`
	TLSServerName      *string `json:"tls_server_name,omitempty"`
	HostHeader         *string `json:"host_header,omitempty"`
	WebSocket          *bool   `json:"websocket,omitempty"`
	DialTimeoutSeconds *int    `json:"dial_timeout_seconds,omitempty"`
	ReadTimeoutSeconds *int    `json:"read_timeout_seconds,omitempty"`
	StripPrefix        *string `json:"strip_prefix,omitempty"`
	Description        *string `json:"description,omitempty"`
	// Confirm acknowledges a private, loopback or control-port target.
	Confirm bool `json:"confirm,omitempty"`
}

func (req standaloneDomainRequest) apply(d *store.StandaloneDomain) {
	setIf(&d.Target, req.Target)
	setIf(&d.UpstreamTLS, req.UpstreamTLS)
	setIf(&d.TLSSkipVerify, req.TLSSkipVerify)
	setIf(&d.TLSServerName, req.TLSServerName)
	setIf(&d.HostHeader, req.HostHeader)
	setIf(&d.WebSocket, req.WebSocket)
	setIf(&d.DialTimeoutSeconds, req.DialTimeoutSeconds)
	setIf(&d.ReadTimeoutSeconds, req.ReadTimeoutSeconds)
	setIf(&d.StripPrefix, req.StripPrefix)
	setIf(&d.Description, req.Description)
}

func sameTarget(a, b string) bool {
	ta, errA := standalone.ParseTarget(a)
	tb, errB := standalone.ParseTarget(b)
	return errA == nil && errB == nil && ta.String() == tb.String()
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func decodeStandaloneRequest(w http.ResponseWriter, r *http.Request) (standaloneDomainRequest, bool) {
	var req standaloneDomainRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, standaloneBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return req, false
	}
	return req, true
}

// handleListStandaloneDomains handles GET /api/v1/domains/standalone
// (optionally ?domain= for one).
func (rt *Router) handleListStandaloneDomains(w http.ResponseWriter, r *http.Request) {
	st := rt.standaloneStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "standalone domains are not supported by this store")
		return
	}
	rows, err := st.ListStandaloneDomains(r.Context())
	if err != nil {
		rt.internalError(w, "api: list standalone domains", err)
		return
	}
	only := normalizeStandaloneDomain(r.URL.Query().Get("domain"))
	out := make([]standaloneDomainResource, 0, len(rows))
	for _, d := range rows {
		if only == "" || d.Domain == only {
			out = append(out, toStandaloneDomainResource(d))
		}
	}
	if only != "" && len(out) == 0 {
		writeError(w, http.StatusNotFound, "standalone domain not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateStandaloneDomain handles POST /api/v1/domains/standalone.
func (rt *Router) handleCreateStandaloneDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.standaloneStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "standalone domains are not supported by this store")
		return
	}
	req, ok := decodeStandaloneRequest(w, r)
	if !ok {
		return
	}
	domain := normalizeStandaloneDomain(req.Domain)
	if msg := validateStandaloneDomainName(domain); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	d := store.StandaloneDomain{Domain: domain, WebSocket: true, CreatedBy: rt.callerDisplayName(r)}
	req.apply(&d)
	if !rt.checkStandaloneConflict(w, r, domain) {
		return
	}
	if !rt.prepareStandalone(w, r, &d, req.Confirm) {
		return
	}
	err := st.CreateStandaloneDomain(r.Context(), d)
	var taken *store.ErrDomainTaken
	switch {
	case errors.As(err, &taken):
		writeError(w, http.StatusConflict, standaloneConflictMessage(domain, describeOwner(taken.Owner)))
		return
	case err != nil:
		rt.internalError(w, "api: create standalone domain", err, slog.String("domain", domain))
		return
	}
	rt.afterStandaloneChange(r, auditActionStandaloneCreated, d)
	saved, err := st.GetStandaloneDomain(r.Context(), domain)
	if err != nil {
		saved = d
	}
	writeJSON(w, http.StatusCreated, toStandaloneDomainResource(saved))
}

// handleUpdateStandaloneDomain handles PUT /api/v1/domains/standalone/{domain}.
func (rt *Router) handleUpdateStandaloneDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.standaloneStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "standalone domains are not supported by this store")
		return
	}
	domain := normalizeStandaloneDomain(r.PathValue("domain"))
	d, err := st.GetStandaloneDomain(r.Context(), domain)
	if errors.Is(err, store.ErrStandaloneDomainNotFound) {
		writeError(w, http.StatusNotFound, "standalone domain not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get standalone domain", err, slog.String("domain", domain))
		return
	}
	req, ok := decodeStandaloneRequest(w, r)
	if !ok {
		return
	}
	if req.Domain != "" && normalizeStandaloneDomain(req.Domain) != domain {
		writeError(w, http.StatusBadRequest, "a standalone domain cannot be renamed; delete it and add the new name")
		return
	}
	previous := d.Target
	req.apply(&d)
	// The target was confirmed when it was saved; only a new one asks again.
	if !rt.prepareStandalone(w, r, &d, req.Confirm || sameTarget(previous, d.Target)) {
		return
	}
	if err := st.UpdateStandaloneDomain(r.Context(), d); err != nil {
		rt.internalError(w, "api: update standalone domain", err, slog.String("domain", domain))
		return
	}
	rt.afterStandaloneChange(r, auditActionStandaloneUpdated, d)
	if saved, err := st.GetStandaloneDomain(r.Context(), domain); err == nil {
		d = saved
	}
	writeJSON(w, http.StatusOK, toStandaloneDomainResource(d))
}

// handleDeleteStandaloneDomain handles DELETE /api/v1/domains/standalone/{domain}.
func (rt *Router) handleDeleteStandaloneDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.standaloneStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "standalone domains are not supported by this store")
		return
	}
	domain := normalizeStandaloneDomain(r.PathValue("domain"))
	d, err := st.GetStandaloneDomain(r.Context(), domain)
	if errors.Is(err, store.ErrStandaloneDomainNotFound) {
		writeError(w, http.StatusNotFound, "standalone domain not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get standalone domain", err, slog.String("domain", domain))
		return
	}
	if err := st.DeleteStandaloneDomain(r.Context(), domain); err != nil && !errors.Is(err, store.ErrStandaloneDomainNotFound) {
		rt.internalError(w, "api: delete standalone domain", err, slog.String("domain", domain))
		return
	}
	rt.afterStandaloneChange(r, auditActionStandaloneDeleted, d)
	w.WriteHeader(http.StatusNoContent)
}

// handleValidateStandaloneDomain handles POST /api/v1/domains/standalone/validate:
// the same checks as create, nothing saved, plus what the server will connect to.
func (rt *Router) handleValidateStandaloneDomain(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStandaloneRequest(w, r)
	if !ok {
		return
	}
	domain := normalizeStandaloneDomain(req.Domain)
	resp := standaloneValidation{Domain: domain}
	if domain != "" {
		if msg := validateStandaloneDomainName(domain); msg != "" {
			resp.DomainError = msg
		} else if owner := rt.standaloneDomainOwner(r.Context(), domain); owner != "" {
			resp.DomainError = standaloneConflictMessage(domain, owner)
		}
	}
	d := store.StandaloneDomain{Domain: domain, WebSocket: true}
	if st := rt.standaloneStore(); st != nil && domain != "" {
		if existing, err := st.GetStandaloneDomain(r.Context(), domain); err == nil {
			d = existing
			resp.DomainError = ""
		}
	}
	req.apply(&d)
	resp.Assessment, resp.TargetURL, resp.TargetError = rt.assessStandalone(r.Context(), &d)
	writeJSON(w, http.StatusOK, resp)
}

type standaloneValidation struct {
	Domain      string                `json:"domain,omitempty"`
	DomainError string                `json:"domain_error,omitempty"`
	TargetURL   string                `json:"target_url,omitempty"`
	TargetError string                `json:"target_error,omitempty"`
	Assessment  standalone.Assessment `json:"assessment"`
}

// prepareStandalone validates d's target and options in place and writes
// the error or confirmation response itself; ok is false when it did.
func (rt *Router) prepareStandalone(w http.ResponseWriter, r *http.Request, d *store.StandaloneDomain, confirm bool) bool {
	a, _, msg := rt.assessStandalone(r.Context(), d)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return false
	}
	if len(a.NeedsConfirmation) > 0 && !confirm {
		writeJSON(w, http.StatusConflict, standaloneConfirmation{
			Error:             "this target needs confirmation: " + strings.Join(a.NeedsConfirmation, "; ") + "; resend with confirm: true",
			NeedsConfirmation: a.NeedsConfirmation,
			Assessment:        a,
		})
		return false
	}
	return true
}

type standaloneConfirmation struct {
	Error             string                `json:"error"`
	NeedsConfirmation []string              `json:"needs_confirmation"`
	Assessment        standalone.Assessment `json:"assessment"`
}

func (rt *Router) afterStandaloneChange(r *http.Request, action string, d store.StandaloneDomain) {
	rt.nudgeReconciler()
	actor := rt.callerDisplayName(r)
	rt.logger.Info("api: standalone domain changed", slog.String("action", action), slog.String("domain", d.Domain),
		slog.String("target", d.Target), slog.String("actor", actor))
	rt.recordStandaloneAudit(r, action, d.Domain)
}
