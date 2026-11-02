package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	parkedBodyLimit = 4 << 10

	auditActionDomainParked   = "domain.parked"
	auditActionDomainMoved    = "domain.moved"
	auditActionDomainAttached = "domain.attached"
	auditActionParkedDeleted  = "domain.parked_deleted"
)

type parkedDomainStore interface {
	ParkServiceDomain(ctx context.Context, app, domain, by, note string) error
	AttachParkedDomainWith(ctx context.Context, domain, app string, restore bool) ([]string, error)
	MoveServiceDomain(ctx context.Context, domain, from, to string) error
	DomainOwnerOf(ctx context.Context, domain string) (string, bool, error)
	GetParkedDomain(ctx context.Context, domain string) (store.ParkedDomain, error)
	ListParkedDomains(ctx context.Context) ([]store.ParkedDomain, error)
	DeleteParkedDomain(ctx context.Context, domain string) error
}

func (rt *Router) parkedStore() parkedDomainStore {
	s, _ := rt.apps.(parkedDomainStore)
	return s
}

// parkedDomainResource is one parked domain on the wire.
type parkedDomainResource struct {
	Domain     string `json:"domain"`
	Kind       string `json:"kind"`
	ParkedFrom string `json:"parked_from,omitempty"`
	Note       string `json:"note,omitempty"`
	ParkedBy   string `json:"parked_by,omitempty"`
	ParkedAt   string `json:"parked_at,omitempty"`
	// HasSettings is true when attach can restore the per-domain settings the park kept.
	HasSettings bool `json:"has_settings,omitempty"`
	// DNS and TLS are the live state, so the list shows whether a parked domain is still worth keeping.
	DNS *parkedDNSState `json:"dns,omitempty"`
	TLS *parkedTLSState `json:"tls,omitempty"`
}

func toParkedDomainResource(p store.ParkedDomain) parkedDomainResource {
	res := parkedDomainResource{Domain: p.Domain, Kind: "parked", ParkedFrom: p.ParkedFrom, Note: p.Note, ParkedBy: p.ParkedBy, HasSettings: p.HasSettings}
	if !p.ParkedAt.IsZero() {
		res.ParkedAt = p.ParkedAt.UTC().Format(time.RFC3339)
	}
	return res
}

// domainMoveRequest is the body of park, move and attach; unused fields stay empty.
type domainMoveRequest struct {
	Domain string `json:"domain,omitempty"`
	App    string `json:"app,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Note   string `json:"note,omitempty"`
	// Confirm acknowledges that traffic for the domain changes owner.
	Confirm bool `json:"confirm,omitempty"`
	// DiscardSettings makes attach drop the settings kept by park instead of restoring them.
	DiscardSettings bool `json:"discard_settings,omitempty"`
}

// domainLifecycleResponse reports what a park, move or attach did, or with
// Applied false what a confirm would do.
type domainLifecycleResponse struct {
	Domain               string `json:"domain"`
	From                 string `json:"from,omitempty"`
	To                   string `json:"to,omitempty"`
	Applied              bool   `json:"applied"`
	RequiresConfirmation bool   `json:"requires_confirmation,omitempty"`
	Message              string `json:"message,omitempty"`
	// RestoredSettings names the per-domain settings kinds an attach restored.
	RestoredSettings []string `json:"restored_settings,omitempty"`
}

func decodeDomainMove(w http.ResponseWriter, r *http.Request) (domainMoveRequest, bool) {
	var req domainMoveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, parkedBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return req, false
	}
	req.Domain = normalizeStandaloneDomain(req.Domain)
	return req, true
}

// handleListParkedDomains handles GET /api/v1/domains/parked.
func (rt *Router) handleListParkedDomains(w http.ResponseWriter, r *http.Request) {
	st := rt.parkedStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "parked domains are not supported by this store")
		return
	}
	rows, err := st.ListParkedDomains(r.Context())
	if err != nil {
		rt.internalError(w, "api: list parked domains", err)
		return
	}
	domains := make([]string, 0, len(rows))
	for _, p := range rows {
		domains = append(domains, p.Domain)
	}
	states := rt.parkedStates(r.Context(), r, domains)
	out := make([]parkedDomainResource, 0, len(rows))
	for _, p := range rows {
		res := toParkedDomainResource(p)
		if st, ok := states[strings.ToLower(p.Domain)]; ok {
			res.DNS, res.TLS = &st.DNS, &st.TLS
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleParkDomain handles POST /api/v1/domains/park: the domain leaves the
// app's list but stays registered, with its DNS and certificate untouched.
func (rt *Router) handleParkDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.parkedStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "parked domains are not supported by this store")
		return
	}
	req, ok := decodeDomainMove(w, r)
	if !ok {
		return
	}
	if req.Domain == "" || req.App == "" {
		writeError(w, http.StatusBadRequest, "domain and app are required")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.App)) {
		return
	}
	before := rt.appDomainList(r.Context(), req.App)
	err := st.ParkServiceDomain(r.Context(), req.App, req.Domain, rt.callerDisplayName(r), strings.TrimSpace(req.Note))
	if !rt.writeLifecycleError(w, err, req.Domain) {
		return
	}
	rt.afterDomainLifecycle(r, auditActionDomainParked, req.Domain, req.App, before)
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: req.Domain, From: req.App, Applied: true})
}

// handleMoveDomain handles POST /api/v1/domains/move. Without confirm it
// only reports the collision check; per-domain settings travel with the domain.
func (rt *Router) handleMoveDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.parkedStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "parked domains are not supported by this store")
		return
	}
	req, ok := decodeDomainMove(w, r)
	if !ok {
		return
	}
	if req.Domain == "" || req.From == "" || req.To == "" || req.From == req.To {
		writeError(w, http.StatusBadRequest, "domain, from and to are required, and from and to must differ")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.From), appRef(req.To)) {
		return
	}
	owner, found, err := st.DomainOwnerOf(r.Context(), req.Domain)
	if err != nil {
		rt.internalError(w, "api: domain owner", err, slog.String("domain", req.Domain))
		return
	}
	if !found || owner != req.From {
		writeError(w, http.StatusConflict, "domain "+req.Domain+" is not served by app "+req.From)
		return
	}
	target, err := rt.apps.GetDesiredService(r.Context(), req.To)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get move target", err, slog.String("app", req.To))
		return
	}
	if slices.Contains(target.Domains, req.Domain) {
		writeError(w, http.StatusConflict, "app "+req.To+" already has domain "+req.Domain)
		return
	}
	if target.Port == 0 {
		writeCodedError(w, http.StatusBadRequest, domainNeedsHTTPPortCode,
			"the target app has no HTTP port, set a port before moving a domain to it")
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusConflict, domainLifecycleResponse{
			Domain: req.Domain, From: req.From, To: req.To, RequiresConfirmation: true,
			Message: "traffic for " + req.Domain + " moves from " + req.From + " to " + req.To + "; resend with confirm: true",
		})
		return
	}
	beforeFrom, beforeTo := rt.appDomainList(r.Context(), req.From), target.Domains
	if !rt.writeLifecycleError(w, st.MoveServiceDomain(r.Context(), req.Domain, req.From, req.To), req.Domain) {
		return
	}
	rt.afterDomainLifecycle(r, auditActionDomainMoved, req.Domain, req.To, beforeTo)
	rt.rerenderDomainEnv(r, req.From, beforeFrom)
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: req.Domain, From: req.From, To: req.To, Applied: true})
}

// handleAttachParkedDomain handles POST /api/v1/domains/parked/{domain}/attach.
func (rt *Router) handleAttachParkedDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.parkedStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "parked domains are not supported by this store")
		return
	}
	req, ok := decodeDomainMove(w, r)
	if !ok {
		return
	}
	domain := normalizeStandaloneDomain(r.PathValue("domain"))
	if req.App == "" {
		writeError(w, http.StatusBadRequest, "app is required")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.App)) {
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), req.App)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get attach target", err, slog.String("app", req.App))
		return
	}
	if svc.Port == 0 {
		writeCodedError(w, http.StatusBadRequest, domainNeedsHTTPPortCode,
			"this app has no HTTP port, set a port before adding a domain: the proxy routes HTTP to it")
		return
	}
	before := svc.Domains
	restored, err := st.AttachParkedDomainWith(r.Context(), domain, req.App, !req.DiscardSettings)
	if !rt.writeLifecycleError(w, err, domain) {
		return
	}
	detail, _ := json.Marshal(map[string]any{"settings_discarded": req.DiscardSettings, "settings_restored": restored})
	rt.afterDomainLifecycle(r, auditActionDomainAttached, domain, req.App, before, string(detail))
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: domain, To: req.App, Applied: true, RestoredSettings: restored})
}

// handleDeleteParkedDomain handles DELETE /api/v1/domains/parked/{domain}. It
// only forgets the registration; DNS records and certificates are left alone.
func (rt *Router) handleDeleteParkedDomain(w http.ResponseWriter, r *http.Request) {
	st := rt.parkedStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "parked domains are not supported by this store")
		return
	}
	domain := normalizeStandaloneDomain(r.PathValue("domain"))
	if !rt.writeLifecycleError(w, st.DeleteParkedDomain(r.Context(), domain), domain) {
		return
	}
	rt.recordDomainLifecycleAudit(r, auditActionParkedDeleted, domain)
	w.WriteHeader(http.StatusNoContent)
}

// appDomainList is the app's default domain list, nil when unreadable.
func (rt *Router) appDomainList(ctx context.Context, app string) []string {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc == nil {
		return nil
	}
	return slices.Clone(svc.Domains)
}

// writeLifecycleError maps store errors and reports whether err was nil.
func (rt *Router) writeLifecycleError(w http.ResponseWriter, err error, domain string) bool {
	var taken *store.ErrDomainTaken
	switch {
	case err == nil:
		return true
	case errors.Is(err, store.ErrParkedDomainNotFound):
		writeError(w, http.StatusNotFound, "parked domain not found")
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
	case errors.Is(err, store.ErrDomainNotOnApp):
		writeError(w, http.StatusBadRequest, "domain "+domain+" is not in that app's domain list")
	case errors.Is(err, store.ErrDomainOnApp):
		writeError(w, http.StatusConflict, "that app already has domain "+domain)
	case errors.As(err, &taken):
		writeError(w, http.StatusConflict, taken.Error())
	default:
		rt.internalError(w, "api: domain lifecycle", err, slog.String("domain", domain))
	}
	return false
}

// afterDomainLifecycle records the app event, re-renders domain-derived env,
// nudges the reconciler and writes the audit entry.
func (rt *Router) afterDomainLifecycle(r *http.Request, action, domain, app string, before []string, detail ...string) {
	next := rt.appDomainList(r.Context(), app)
	if ev, ok := domainChangeEvent(app, before, next); ok {
		rt.recordAppEvent(r, ev)
	}
	rt.rerenderDomainEnv(r, app, before)
	rt.nudgeReconciler()
	rt.recordDomainLifecycleAudit(r, action, domain, detail...)
}

func (rt *Router) recordDomainLifecycleAudit(r *http.Request, action, domain string, detail ...string) {
	pt, actorID, _, err := rt.callerPrincipal(r)
	if err != nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: domain lifecycle audit id", slog.String("domain", domain), slog.String("error", err.Error()))
		return
	}
	at := auditActorType(pt)
	entry := store.AuditEntry{
		ID: id, ActorType: at, ActorID: actorID, ActorName: rt.auditActorName(r.Context(), at, actorID, ""),
		Ability: AbilityWriteSensitive, Method: r.Method, Path: r.URL.Path,
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action,
	}
	if len(detail) > 0 {
		entry.Detail = detail[0]
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: save domain lifecycle audit entry", slog.String("domain", domain), slog.String("error", err.Error()))
	}
}
