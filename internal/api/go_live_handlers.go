package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

const (
	goLiveBodyLimit        = 16 << 10
	defaultAutomationRuns  = 20
	maxAutomationRuns      = 100
	goLivePlanDomainsLimit = 20
)

type automationRunStore interface {
	GetDomainAutomationRun(ctx context.Context, id string) (store.DomainAutomationRun, error)
	ListDomainAutomationRuns(ctx context.Context, limit int) ([]store.DomainAutomationRun, error)
	MarkDomainAutomationRunUndone(ctx context.Context, id string, at time.Time) error
}

func (rt *Router) automationRuns() automationRunStore {
	s, _ := rt.apps.(automationRunStore)
	return s
}

func (rt *Router) loadGoLiveApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return "", false
		}
		rt.internalError(w, "api: go-live: load app failed", err, slog.String("name", name))
		return "", false
	}
	return name, true
}

func decodeGoLiveRequest(w http.ResponseWriter, r *http.Request) (goLiveRequest, bool) {
	var req goLiveRequest
	if r.ContentLength == 0 {
		return req, true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, goLiveBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	if req.Automation != nil {
		if msg := req.Automation.validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return req, false
		}
	}
	switch req.DNS {
	case "", dnsModeAuto, dnsModeOff, dnsModePreview:
	default:
		writeError(w, http.StatusBadRequest, "dns must be auto, off or preview")
		return req, false
	}
	return req, true
}

// handleGoLiveStatus handles GET .../domains/{domain}/go-live: the poll
// friendly current state, never a write, bounded by goLiveStatusBudget.
func (rt *Router) handleGoLiveStatus(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadGoLiveApp(w, r)
	if !ok {
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	owns, err := rt.appOwnsDomain(r.Context(), name, domain)
	if err != nil {
		rt.internalError(w, "api: go-live status: ownership check failed", err)
		return
	}
	if !owns {
		writeError(w, http.StatusNotFound, "domain is not one of this app's domains")
		return
	}
	policy := rt.effectiveAutomation(r.Context(), r, nil)
	writeJSON(w, http.StatusOK, rt.goLive(r.Context(), r, name, domain, policy, goLiveRequest{}, goLiveModeStatus))
}

// handleGoLiveRun handles POST .../domains/{domain}/go-live (root): attaches
// the domain when needed, then runs the automation and one verification pass.
func (rt *Router) handleGoLiveRun(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadGoLiveApp(w, r)
	if !ok {
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if err := ingress.ValidateWildcardDomain(domain); err != nil || domain == "" {
		writeError(w, http.StatusBadRequest, "invalid domain")
		return
	}
	req, ok := decodeGoLiveRequest(w, r)
	if !ok {
		return
	}
	var undo []undoAction
	owns, err := rt.appOwnsDomain(r.Context(), name, domain)
	if err != nil {
		rt.internalError(w, "api: go-live: ownership check failed", err)
		return
	}
	if !owns {
		editor, ok := rt.apps.(serviceDomainsEditor)
		if !ok {
			writeError(w, http.StatusNotImplemented, "editing domains is not supported by this store")
			return
		}
		_, _, err := editor.EditServiceDomains(r.Context(), name, func(cur []string) ([]string, error) {
			return applyDomainEdit(cur, []string{domain}, nil)
		})
		var taken *store.ErrDomainTaken
		switch {
		case errors.As(err, &taken):
			writeError(w, http.StatusConflict, taken.Error())
			return
		case err != nil:
			rt.internalError(w, "api: go-live: attach domain failed", err)
			return
		}
		undo = append(undo, undoAction{Kind: undoKindAppDomain, App: name, Domain: domain})
		rt.nudgeReconciler()
	}
	policy := rt.effectiveAutomation(r.Context(), r, req.Automation)
	res := rt.goLive(r.Context(), r, name, domain, policy, req, goLiveModeApply)
	res.undo = append(undo, res.undo...)
	res.Undoable = len(res.undo) > 0
	rt.recordGoLiveRun(r.Context(), &res)
	writeJSON(w, http.StatusOK, res)
}

type goLivePlanRequest struct {
	Domain     string                    `json:"domain"`
	Domains    []string                  `json:"domains,omitempty"`
	Automation *domainAutomationOverride `json:"automation,omitempty"`
}

type goLivePlanResponse struct {
	Plans []goLiveResult `json:"plans"`
}

// handleGoLivePlan handles POST .../domains/go-live/plan: the dry run of the
// whole flow, no writes and no probes.
func (rt *Router) handleGoLivePlan(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadGoLiveApp(w, r)
	if !ok {
		return
	}
	var req goLivePlanRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, goLiveBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Automation != nil {
		if msg := req.Automation.validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	domains := normalizeDomainList(append([]string{req.Domain}, req.Domains...))
	if len(domains) == 0 || len(domains) > goLivePlanDomainsLimit {
		writeError(w, http.StatusBadRequest, "provide between 1 and "+strconv.Itoa(goLivePlanDomainsLimit)+" domains")
		return
	}
	policy := rt.effectiveAutomation(r.Context(), r, req.Automation)
	out := goLivePlanResponse{Plans: make([]goLiveResult, 0, len(domains))}
	for _, d := range domains {
		if err := ingress.ValidateWildcardDomain(d); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		out.Plans = append(out.Plans, rt.goLive(r.Context(), r, name, d, policy, goLiveRequest{}, goLiveModePlan))
	}
	writeJSON(w, http.StatusOK, out)
}

type automationRunResource struct {
	ID        string       `json:"id"`
	App       string       `json:"app"`
	Domain    string       `json:"domain"`
	Result    string       `json:"result"`
	Steps     []goLiveStep `json:"steps"`
	CreatedAt time.Time    `json:"created_at"`
	UndoneAt  *time.Time   `json:"undone_at,omitempty"`
	Undoable  bool         `json:"undoable"`
}

func toAutomationRunResource(run store.DomainAutomationRun) automationRunResource {
	var steps []goLiveStep
	_ = json.Unmarshal([]byte(run.Steps), &steps)
	var undo []undoAction
	_ = json.Unmarshal([]byte(run.Undo), &undo)
	return automationRunResource{
		ID: run.ID, App: run.AppName, Domain: run.Domain, Result: run.Result, Steps: steps,
		CreatedAt: run.CreatedAt, UndoneAt: run.UndoneAt, Undoable: run.UndoneAt == nil && len(undo) > 0,
	}
}

// handleListAutomationRuns handles GET /api/v1/domains/automation/runs.
func (rt *Router) handleListAutomationRuns(w http.ResponseWriter, r *http.Request) {
	st := rt.automationRuns()
	if st == nil {
		writeJSON(w, http.StatusOK, []automationRunResource{})
		return
	}
	limit := defaultAutomationRuns
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, maxAutomationRuns)
	}
	runs, err := st.ListDomainAutomationRuns(r.Context(), limit)
	if err != nil {
		rt.internalError(w, "api: list automation runs", err)
		return
	}
	out := make([]automationRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, toAutomationRunResource(run))
	}
	writeJSON(w, http.StatusOK, out)
}

type undoResponse struct {
	automationRunResource
	Reverted []string `json:"reverted"`
	Skipped  []string `json:"skipped,omitempty"`
}

// handleUndoAutomationRun handles POST /api/v1/domains/automation/runs/{id}/undo
// (root): reverses only what that run created.
func (rt *Router) handleUndoAutomationRun(w http.ResponseWriter, r *http.Request) {
	st := rt.automationRuns()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "automation history is not supported by this store")
		return
	}
	run, err := st.GetDomainAutomationRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrAutomationRunNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get automation run", err)
		return
	}
	if run.UndoneAt != nil {
		writeError(w, http.StatusConflict, "this run was already undone")
		return
	}
	var undo []undoAction
	if err := json.Unmarshal([]byte(run.Undo), &undo); err != nil {
		rt.internalError(w, "api: decode undo actions", err)
		return
	}
	resp := undoResponse{Reverted: []string{}}
	for i := len(undo) - 1; i >= 0; i-- {
		label, err := rt.undoOne(r.Context(), r, run.AppName, undo[i])
		if err != nil {
			rt.logger.Warn("api: undo action failed", slog.String("error", err.Error()), slog.String("domain", undo[i].Domain), slog.String("kind", undo[i].Kind))
			resp.Skipped = append(resp.Skipped, label)
			continue
		}
		resp.Reverted = append(resp.Reverted, label)
	}
	now := time.Now()
	if err := st.MarkDomainAutomationRunUndone(r.Context(), run.ID, now); err != nil {
		rt.logger.Warn("api: mark run undone failed", slog.String("error", err.Error()), slog.String("run", run.ID))
	}
	run.UndoneAt = &now
	resp.automationRunResource = toAutomationRunResource(run)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) undoOne(ctx context.Context, r *http.Request, runApp string, u undoAction) (string, error) {
	switch u.Kind {
	case undoKindDNSRecord, undoKindDNSRestore:
		t, err := rt.resolveDNSTarget(ctx, u.Domain)
		if err != nil || t == nil {
			return "DNS record " + u.Name, errors.New("DNS provider unavailable")
		}
		rr := libdns.RR{Name: u.Name, Type: u.RecordType, Data: u.Value, TTL: time.Duration(u.TTLSeconds) * time.Second}
		if u.Kind == undoKindDNSRestore {
			if _, err := t.Manager.AppendRecords(ctx, t.Zone, []libdns.Record{rr}); err != nil {
				return "restore DNS record " + u.Name, err
			}
			rt.recordDNSAudit(ctx, r, store.AuditActionDNSRecordCreated, runApp, u.Domain, rr)
			return "restored the previous " + u.RecordType + " record for " + u.Domain, nil
		}
		if err := deleteExactRecord(ctx, t, rr); err != nil {
			return "DNS record " + u.Name, err
		}
		if ms := rt.managedDNS(); ms != nil {
			_ = ms.DeleteManagedDNSRecord(ctx, u.Domain, u.RecordType)
		}
		rt.recordDNSAudit(ctx, r, store.AuditActionDNSRecordDeleted, runApp, u.Domain, rr)
		return "removed the " + u.RecordType + " record for " + u.Domain, nil
	case undoKindAppDomain:
		editor, ok := rt.apps.(serviceDomainsEditor)
		if !ok {
			return "detach " + u.Domain, errors.New("not supported")
		}
		_, _, err := editor.EditServiceDomains(ctx, u.App, func(cur []string) ([]string, error) {
			return applyDomainEdit(cur, nil, []string{u.Domain})
		})
		if err != nil && !errors.Is(err, errDomainNotSet) {
			return "detach " + u.Domain, err
		}
		return "detached " + u.Domain, nil
	case undoKindRedirect:
		if err := rt.domainRedirect.DeleteDomainRedirect(ctx, u.Domain); err != nil {
			return "redirect " + u.Domain, err
		}
		return "removed the redirect for " + u.Domain, nil
	}
	return u.Kind, errors.New("unknown action")
}

type dnsZoneResponse struct {
	Domain     string `json:"domain"`
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
	Found      bool   `json:"found"`
	Zone       string `json:"zone,omitempty"`
	Message    string `json:"message,omitempty"`
}

// handleDNSZone handles GET /api/v1/dns/zone?domain=: whether a connected DNS
// provider manages the domain's zone, for the base-domain field's live hint.
func (rt *Router) handleDNSZone(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if err := validateHostname(domain); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rt.zoneLookup(r.Context(), domain))
}

func (rt *Router) zoneLookup(ctx context.Context, domain string) dnsZoneResponse {
	out := dnsZoneResponse{Domain: domain, Provider: rt.activeDNSProvider(ctx)}
	t, err := rt.resolveDNSTarget(ctx, domain)
	switch {
	case errors.Is(err, dnsrecords.ErrZoneNotFound):
		out.Configured = true
		out.Message = "the connected DNS provider does not manage a zone for this domain"
	case err != nil:
		out.Message = "could not reach the DNS provider"
	case t == nil:
		out.Message = "no DNS provider is connected"
	default:
		out.Configured, out.Found, out.Zone, out.Provider = true, true, strings.TrimSuffix(t.Zone, "."), t.Provider
	}
	return out
}
