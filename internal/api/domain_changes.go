package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

const (
	domainChangesLimit     = 50
	defaultRollbackMinutes = 60
	rollbackReverted       = "reverted"
	rollbackSkipped        = "skipped"
)

type domainChangeEffect struct {
	Action      string `json:"action"`
	Description string `json:"description"`
}

type domainChange struct {
	RunID         string               `json:"run_id"`
	Kind          string               `json:"kind"`
	App           string               `json:"app,omitempty"`
	Result        string               `json:"result"`
	CreatedAt     time.Time            `json:"created_at"`
	Summary       []domainChangeEffect `json:"summary"`
	RollbackUntil time.Time            `json:"rollback_until"`
	CanRollback   bool                 `json:"can_rollback"`
	AlreadyUndone bool                 `json:"already_undone"`
}

func (rt *Router) rollbackWindow(r *http.Request) time.Duration {
	n := defaultRollbackMinutes
	if rt.runtimeSettings != nil {
		if v := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDomainsRollbackWindowMinutes); v > 0 {
			n = v
		}
	}
	return time.Duration(n) * time.Minute
}

func describeUndo(u undoAction) domainChangeEffect {
	switch u.Kind {
	case undoKindDNSRecord:
		return domainChangeEffect{Action: "created", Description: fmt.Sprintf("%s record %s -> %s", u.RecordType, u.Name, u.Value)}
	case undoKindDNSRestore:
		return domainChangeEffect{Action: "replaced", Description: fmt.Sprintf("%s record %s (was %s)", u.RecordType, u.Name, u.Value)}
	case undoKindAppDomain:
		return domainChangeEffect{Action: "attached", Description: "domain " + u.Domain + " attached to app " + u.App}
	case undoKindMoveDomain:
		return domainChangeEffect{Action: "moved", Description: "domain " + u.Domain + " moved from app " + u.App + " to app " + u.To}
	case undoKindRedirect:
		return domainChangeEffect{Action: "created", Description: "redirect for " + u.Domain}
	}
	return domainChangeEffect{Action: "changed", Description: u.Kind}
}

func (rt *Router) toDomainChange(run store.DomainAutomationRun, window time.Duration, now time.Time) domainChange {
	var undo []undoAction
	_ = json.Unmarshal([]byte(run.Undo), &undo)
	var steps []goLiveStep
	_ = json.Unmarshal([]byte(run.Steps), &steps)
	kind := "dns_apply"
	if len(steps) > 0 {
		kind = "go_live"
	}
	if len(undo) > 0 && undo[0].Kind == undoKindMoveDomain {
		kind = "domain_move"
	}
	until := run.CreatedAt.Add(window)
	c := domainChange{
		RunID: run.ID, Kind: kind, App: run.AppName, Result: run.Result, CreatedAt: run.CreatedAt,
		Summary: []domainChangeEffect{}, RollbackUntil: until, AlreadyUndone: run.UndoneAt != nil,
	}
	for _, u := range undo {
		c.Summary = append(c.Summary, describeUndo(u))
	}
	c.CanRollback = !c.AlreadyUndone && len(undo) > 0 && now.Before(until)
	return c
}

// handleListDomainChanges handles GET /api/v1/domains/{domain}/changes.
func (rt *Router) handleListDomainChanges(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if err := validateHostname(domain); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := []domainChange{}
	st := rt.automationRuns()
	if st == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	runs, err := st.ListDomainAutomationRunsForDomain(r.Context(), domain, domainChangesLimit)
	if err != nil {
		rt.internalError(w, "api: list domain changes", err)
		return
	}
	canSeeApp, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list domain changes: visibility", err)
		return
	}
	window, now := rt.rollbackWindow(r), time.Now()
	for _, run := range runs {
		if run.AppName != "" && !canSeeApp(run.AppName) {
			continue
		}
		out = append(out, rt.toDomainChange(run, window, now))
	}
	writeJSON(w, http.StatusOK, out)
}

type rollbackOutcome struct {
	Status  string `json:"status"`
	Label   string `json:"label"`
	Message string `json:"message,omitempty"`
}

type rollbackResponse struct {
	RunID    string            `json:"run_id"`
	Domain   string            `json:"domain"`
	UndoneAt time.Time         `json:"undone_at"`
	Outcomes []rollbackOutcome `json:"outcomes"`
}

// recordChangedSince reports a created DNS record whose value no longer
// matches what the run wrote, so a newer edit is never deleted.
func (rt *Router) recordChangedSince(r *http.Request, u undoAction) (string, bool) {
	t, err := rt.resolveDNSTarget(r.Context(), u.Domain)
	if err != nil || t == nil {
		return "", false
	}
	live, err := t.Manager.GetRecords(r.Context(), t.Zone)
	if err != nil {
		return "", false
	}
	norm := func(s string) string { return strings.ToLower(strings.TrimSuffix(s, ".")) }
	var other []libdns.RR
	for _, rec := range live {
		rr := rec.RR()
		if !strings.EqualFold(rr.Name, u.Name) || rr.Type != u.RecordType {
			continue
		}
		if norm(rr.Data) == norm(u.Value) {
			return "", false
		}
		other = append(other, rr)
	}
	if len(other) == 0 {
		return "", false
	}
	return "the " + u.RecordType + " record " + u.Name + " now points to " + other[0].Data + ", changed since this run wrote " + u.Value + "; left alone", true
}

// handleRollbackDomainChange handles POST /api/v1/domains/{domain}/changes/{id}/rollback (root).
func (rt *Router) handleRollbackDomainChange(w http.ResponseWriter, r *http.Request) {
	st := rt.automationRuns()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "automation history is not supported by this store")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	run, err := st.GetDomainAutomationRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrAutomationRunNotFound) || (err == nil && !strings.EqualFold(run.Domain, domain)) {
		writeError(w, http.StatusNotFound, "change not found for this domain")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get domain change", err)
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "change not found for this domain"), appRef(run.AppName)) {
		return
	}
	if run.UndoneAt != nil {
		writeError(w, http.StatusConflict, "this change was already rolled back")
		return
	}
	window, now := rt.rollbackWindow(r), time.Now()
	if until := run.CreatedAt.Add(window); !now.Before(until) {
		writeError(w, http.StatusConflict, fmt.Sprintf("the rollback window closed at %s (%d minutes after the change); edit the records by hand instead", until.UTC().Format(time.RFC3339), int(window/time.Minute)))
		return
	}
	var undo []undoAction
	if err := json.Unmarshal([]byte(run.Undo), &undo); err != nil {
		rt.internalError(w, "api: decode undo actions", err)
		return
	}
	if len(undo) == 0 {
		writeError(w, http.StatusConflict, "this change created nothing that can be rolled back")
		return
	}
	resp := rollbackResponse{RunID: run.ID, Domain: domain, Outcomes: []rollbackOutcome{}}
	moveRefused := false
	for i := len(undo) - 1; i >= 0; i-- {
		if undo[i].Kind == undoKindDNSRecord {
			if msg, changed := rt.recordChangedSince(r, undo[i]); changed {
				resp.Outcomes = append(resp.Outcomes, rollbackOutcome{Status: rollbackSkipped, Label: "DNS record " + undo[i].Name, Message: msg})
				continue
			}
		}
		label, err := rt.undoOne(r.Context(), r, run.AppName, undo[i])
		if err != nil {
			rt.logger.Warn("api: rollback action failed", slog.String("error", err.Error()), slog.String("domain", undo[i].Domain), slog.String("kind", undo[i].Kind))
			resp.Outcomes = append(resp.Outcomes, rollbackOutcome{Status: rollbackSkipped, Label: label, Message: "could not be reverted"})
			moveRefused = moveRefused || undo[i].Kind == undoKindMoveDomain
			continue
		}
		resp.Outcomes = append(resp.Outcomes, rollbackOutcome{Status: rollbackReverted, Label: label})
	}
	resp.UndoneAt = time.Now()
	if moveRefused {
		// Nothing moved, so the change stays rollbackable once the conflict is cleared.
		rt.nudgeReconciler()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if err := st.MarkDomainAutomationRunUndone(r.Context(), run.ID, resp.UndoneAt); err != nil {
		rt.logger.Warn("api: mark change rolled back failed", slog.String("error", err.Error()), slog.String("run", run.ID))
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, resp)
}
