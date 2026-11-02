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
	ParkDomainFrom(ctx context.Context, src store.DomainTarget, domain, by, note string) error
	AttachParkedDomainTo(ctx context.Context, domain string, dst store.DomainTarget, restore bool) ([]string, error)
	MoveDomainBetweenTargets(ctx context.Context, domain string, from, to store.DomainTarget) error
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
	// ParkedFromKind is app or static_site: what ParkedFrom names.
	ParkedFromKind string `json:"parked_from_kind,omitempty"`
	// ParkedFromEnvironment is the environment the domain left; empty for the app's default list.
	ParkedFromEnvironment string `json:"parked_from_environment,omitempty"`
	Note                  string `json:"note,omitempty"`
	ParkedBy              string `json:"parked_by,omitempty"`
	ParkedAt              string `json:"parked_at,omitempty"`
	// HasSettings is true when attach can restore the per-domain settings the park kept.
	HasSettings bool `json:"has_settings,omitempty"`
	// DNS and TLS are the live state, so the list shows whether a parked domain is still worth keeping.
	DNS *parkedDNSState `json:"dns,omitempty"`
	TLS *parkedTLSState `json:"tls,omitempty"`
}

func toParkedDomainResource(p store.ParkedDomain) parkedDomainResource {
	res := parkedDomainResource{Domain: p.Domain, Kind: "parked", ParkedFrom: p.ParkedFrom, ParkedFromKind: kindOrAppName(p.ParkedFromKind), Note: p.Note, ParkedBy: p.ParkedBy, HasSettings: p.HasSettings}
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
	// Environment is the environment domain set for park and attach; empty means the app's default list.
	Environment string `json:"environment,omitempty"`
	// FromEnvironment and ToEnvironment pick the source and target sets of a move.
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
	// TargetKind is app (default) or static_site for park and attach; the name is in App.
	TargetKind string `json:"target_kind,omitempty"`
	// FromKind and ToKind are app (default) or static_site for the two names of a move.
	FromKind string `json:"from_kind,omitempty"`
	ToKind   string `json:"to_kind,omitempty"`
	// Confirm acknowledges that traffic for the domain changes owner.
	Confirm bool `json:"confirm,omitempty"`
	// DiscardSettings makes attach drop the settings kept by park instead of restoring them.
	DiscardSettings bool `json:"discard_settings,omitempty"`
}

// domainLifecycleResponse reports what a park, move or attach did, or with
// Applied false what a confirm would do.
type domainLifecycleResponse struct {
	Domain string `json:"domain"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	// Kind, FromKind and ToKind echo whether each name is an app or a static_site.
	Kind     string `json:"kind,omitempty"`
	FromKind string `json:"from_kind,omitempty"`
	ToKind   string `json:"to_kind,omitempty"`
	// Environment, FromEnvironment and ToEnvironment echo the resolved environment names.
	Environment          string `json:"environment,omitempty"`
	FromEnvironment      string `json:"from_environment,omitempty"`
	ToEnvironment        string `json:"to_environment,omitempty"`
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
	envNames := rt.environmentNamesByID(r.Context())
	out := make([]parkedDomainResource, 0, len(rows))
	for _, p := range rows {
		res := toParkedDomainResource(p)
		if p.ParkedFromEnvironment != "" {
			res.ParkedFromEnvironment = envNameOrID(envNames, p.ParkedFromEnvironment)
		}
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
	side, ok := rt.resolveDomainTarget(w, r, req.TargetKind, req.App, req.Environment)
	if !ok {
		return
	}
	err := st.ParkDomainFrom(r.Context(), side.target, req.Domain, rt.callerDisplayName(r), strings.TrimSpace(req.Note))
	if !rt.writeLifecycleError(w, err, req.Domain) {
		return
	}
	detail, _ := json.Marshal(map[string]string{"kind": side.target.Kind, "environment": side.envName})
	rt.afterDomainLifecycle(r, auditActionDomainParked, req.Domain, string(detail), side)
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: req.Domain, From: req.App, Kind: side.target.Kind, Environment: side.envName, Applied: true})
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
	fromKind, okFrom := normalizeDomainKind(req.FromKind)
	toKind, okTo := normalizeDomainKind(req.ToKind)
	if !okFrom || !okTo {
		writeError(w, http.StatusBadRequest, "from_kind and to_kind must be app or static_site")
		return
	}
	sameSet := req.From == req.To && fromKind == toKind && strings.EqualFold(strings.TrimSpace(req.FromEnvironment), strings.TrimSpace(req.ToEnvironment))
	if req.Domain == "" || req.From == "" || req.To == "" || sameSet {
		writeError(w, http.StatusBadRequest, "domain, from and to are required, and the source and target must differ")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.From), appRef(req.To)) {
		return
	}
	from, ok := rt.resolveDomainTarget(w, r, fromKind, req.From, req.FromEnvironment)
	if !ok {
		return
	}
	to, ok := rt.resolveDomainTarget(w, r, toKind, req.To, req.ToEnvironment)
	if !ok {
		return
	}
	if !slices.Contains(from.set, req.Domain) {
		writeError(w, http.StatusConflict, "domain "+req.Domain+" is not served by "+describeDomainTarget(from))
		return
	}
	if slices.Contains(to.set, req.Domain) {
		writeError(w, http.StatusConflict, describeDomainTarget(to)+" already has domain "+req.Domain)
		return
	}
	if toKind == store.DomainKindApp && !rt.appHasHTTPPort(w, r, req.To, "the target app has no HTTP port, set a port before moving a domain to it") {
		return
	}
	if !req.Confirm {
		msg := "traffic for " + req.Domain + " moves from " + describeDomainTarget(from) + " to " + describeDomainTarget(to) + "; resend with confirm: true"
		if fromKind != toKind {
			msg += ". Its per-domain settings are not kept across a static site boundary"
		}
		writeJSON(w, http.StatusConflict, domainLifecycleResponse{
			Domain: req.Domain, From: req.From, To: req.To, FromKind: fromKind, ToKind: toKind, FromEnvironment: from.envName, ToEnvironment: to.envName,
			RequiresConfirmation: true, Message: msg,
		})
		return
	}
	if !rt.writeLifecycleError(w, st.MoveDomainBetweenTargets(r.Context(), req.Domain, from.target, to.target), req.Domain) {
		return
	}
	detail, _ := json.Marshal(map[string]string{"from": req.From, "to": req.To, "from_kind": fromKind, "to_kind": toKind, "from_environment": from.envName, "to_environment": to.envName})
	rt.afterDomainLifecycle(r, auditActionDomainMoved, req.Domain, string(detail), to, from)
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: req.Domain, From: req.From, To: req.To, FromKind: fromKind, ToKind: toKind, FromEnvironment: from.envName, ToEnvironment: to.envName, Applied: true})
}

// appHasHTTPPort writes the coded 400 and reports false when the app has no port.
func (rt *Router) appHasHTTPPort(w http.ResponseWriter, r *http.Request, app, msg string) bool {
	svc, err := rt.apps.GetDesiredService(r.Context(), app)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: get domain target", err, slog.String("app", app))
		return false
	}
	if svc.Port == 0 {
		writeCodedError(w, http.StatusBadRequest, domainNeedsHTTPPortCode, msg)
		return false
	}
	return true
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
	kind, ok := normalizeDomainKind(req.TargetKind)
	if !ok {
		writeError(w, http.StatusBadRequest, "kind must be app or static_site")
		return
	}
	if kind == store.DomainKindApp && !rt.appHasHTTPPort(w, r, req.App,
		"this app has no HTTP port, set a port before adding a domain: the proxy routes HTTP to it") {
		return
	}
	side, ok := rt.resolveDomainTarget(w, r, kind, req.App, req.Environment)
	if !ok {
		return
	}
	var note string
	if kind == store.DomainKindStaticSite && !req.DiscardSettings {
		if p, err := st.GetParkedDomain(r.Context(), domain); err == nil && p.HasSettings {
			note = "the settings kept by park were not restored: a static site has no per-domain settings"
		}
	}
	restored, err := st.AttachParkedDomainTo(r.Context(), domain, side.target, !req.DiscardSettings)
	if !rt.writeLifecycleError(w, err, domain) {
		return
	}
	detail, _ := json.Marshal(map[string]any{"kind": kind, "environment": side.envName, "settings_discarded": req.DiscardSettings, "settings_restored": restored})
	rt.afterDomainLifecycle(r, auditActionDomainAttached, domain, string(detail), side)
	writeJSON(w, http.StatusOK, domainLifecycleResponse{Domain: domain, To: req.App, Kind: kind, Environment: side.envName, Applied: true, RestoredSettings: restored, Message: note})
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
	case errors.Is(err, store.ErrEnvironmentNotFound):
		writeError(w, http.StatusNotFound, "environment not found")
	case errors.Is(err, store.ErrDomainNotOnApp):
		writeError(w, http.StatusBadRequest, "domain "+domain+" is not in that domain list")
	case errors.Is(err, store.ErrDomainOnApp):
		writeError(w, http.StatusConflict, "that app already has domain "+domain)
	case errors.As(err, &taken):
		writeError(w, http.StatusConflict, taken.Error())
	default:
		rt.internalError(w, "api: domain lifecycle", err, slog.String("domain", domain))
	}
	return false
}

// afterDomainLifecycle records the app event for each touched domain set
// (env-only changes included), re-renders domain-derived env, nudges the
// reconciler and writes the audit entry.
func (rt *Router) afterDomainLifecycle(r *http.Request, action, domain, detail string, sides ...lifecycleSide) {
	for _, side := range sides {
		next := rt.domainSetList(r.Context(), side.target)
		if ev, ok := domainChangeEvent(side.target.Name, side.set, next); ok {
			if side.envName != "" {
				ev.Detail += " in environment " + side.envName
			}
			rt.recordAppEvent(r, ev)
		}
		if side.target.Kind != store.DomainKindStaticSite {
			rt.rerenderDomainEnv(r, side.target.Name, side.def)
		}
	}
	rt.nudgeReconciler()
	rt.recordDomainLifecycleAudit(r, action, domain, detail)
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

// resolveLifecycleEnv maps an environment ID, name or kind to its ID for an
// app's domain set. An empty value is the default list (empty ID and name).
func (rt *Router) resolveLifecycleEnv(w http.ResponseWriter, r *http.Request, app, value string) (id, name string, ok bool) {
	if strings.TrimSpace(value) == "" {
		return "", "", true
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), app)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", "", false
	}
	if err != nil {
		rt.internalError(w, "api: get app for environment", err, slog.String("app", app))
		return "", "", false
	}
	env, err := rt.resolveDomainEnvironment(r.Context(), value, svc.ProjectID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found: "+value)
		return "", "", false
	}
	if err != nil {
		rt.internalError(w, "api: resolve environment", err, slog.String("app", app))
		return "", "", false
	}
	return env.ID, env.Name, true
}

// domainSetOf is the app's default list, or its envID set when envID is set.
func (rt *Router) domainSetOf(ctx context.Context, svc *store.DesiredService, envID string) []string {
	if envID == "" {
		return svc.Domains
	}
	lister, ok := rt.apps.(serviceEnvironmentDomainStore)
	if !ok {
		return nil
	}
	sets, err := lister.ListServiceEnvironmentDomains(ctx, svc.Name)
	if err != nil {
		return nil
	}
	return sets[envID]
}

// environmentNamesByID is best effort: a failed read leaves IDs unresolved.
func (rt *Router) environmentNamesByID(ctx context.Context) map[string]string {
	out := map[string]string{}
	if rt.environments == nil {
		return out
	}
	envs, err := rt.environments.ListAllEnvironments(ctx)
	if err != nil {
		return out
	}
	for _, e := range envs {
		out[e.ID] = e.Name
	}
	return out
}

func envNameOrID(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}

func kindOrAppName(kind string) string {
	if kind == "" {
		return store.DomainKindApp
	}
	return kind
}
