package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

type wwwApplyRequest struct {
	Mode      string `json:"mode"`
	Canonical string `json:"canonical"`
	Replace   bool   `json:"replace"`
	Confirm   bool   `json:"confirm"`
	// Redirect is a pointer so an absent field keeps the redirect; only an explicit false skips it.
	Redirect *bool `json:"redirect,omitempty"`
}

// joinNote appends a note to a message that may be empty.
func joinNote(msg, note string) string {
	if msg == "" {
		return note
	}
	return msg + "; " + note
}

const wwwNoRedirectNote = "no redirect: both hosts serve the app"

// Per-record apply results beyond the dnsrecords outcomes.
const (
	wwwResultFailed       = "failed"
	wwwResultNotAttempted = "not_attempted"
)

type wwwApplyRecord struct {
	wwwPlanRecord
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

type wwwApplyResponse struct {
	Domain          string           `json:"domain"`
	Provider        string           `json:"provider"`
	Zone            string           `json:"zone"`
	Mode            string           `json:"mode"`
	Canonical       string           `json:"canonical"`
	Applied         bool             `json:"applied"`
	Records         []wwwApplyRecord `json:"records"`
	Redirect        *wwwPlanRedirect `json:"redirect"`
	RedirectApplied bool             `json:"redirect_applied"`
	RedirectMessage string           `json:"redirect_message,omitempty"`
	Undo            []undoAction     `json:"undo"`
	RunID           string           `json:"run_id,omitempty"`
	Message         string           `json:"message,omitempty"`
}

// handleApplyDomainWWW writes the apex and www records the plan lists. A
// conflict is reported, never overwritten without replace, and the first
// failed write stops the run with the done and undone records listed.
func (rt *Router) handleApplyDomainWWW(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req wwwApplyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.callerIsRoot(r) {
		writeError(w, http.StatusForbidden, "writing DNS records needs the root ability; use the plan and add the records manually, or ask an admin")
		return
	}
	st, ok := rt.buildWWWPlan(w, r, domain, req.Mode, req.Canonical, req.Replace)
	if !ok {
		return
	}
	if st.target == nil {
		writeError(w, http.StatusConflict, "no DNS provider manages this domain's zone; nothing can be written")
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirm is required to write DNS; this would change: "+wwwPreviewSummary(st.items))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), dnsAutomationBudget())
	defer cancel()

	app := r.PathValue("name")
	skipRedirect := req.Redirect != nil && !*req.Redirect
	if skipRedirect {
		st.res.Redirect = nil
	}
	out := wwwApplyResponse{Domain: domain, Provider: st.res.Provider, Zone: st.res.Zone, Mode: st.res.Mode,
		Canonical: st.res.Canonical, Records: []wwwApplyRecord{}, Redirect: st.res.Redirect, Undo: []undoAction{}}
	failed, conflicts := false, false
	wwwHost := "www." + domain
	foreignWWW := rt.domainOwner(ctx, wwwHost)
	if foreignWWW == app {
		foreignWWW = ""
	}
	for i, it := range st.items {
		rec := wwwApplyRecord{wwwPlanRecord: st.res.Records[i], Result: it.Plan.Outcome}
		// The www record is tracked under its own host so it never shares a key with the apex row.
		trackDomain := domain
		isWWW := it.Desired.Name == "www"
		if isWWW {
			trackDomain = wwwHost
		}
		switch {
		case failed:
			rec.Result = wwwResultNotAttempted
		case isWWW && foreignWWW != "" && it.Plan.Outcome != dnsrecords.OutcomeUnchanged:
			conflicts = true
			rec.Result = dnsrecords.OutcomeConflict
			rec.Message = wwwHost + " belongs to app " + foreignWWW + "; it was left alone"
		case it.Plan.Outcome == dnsrecords.OutcomeConflict:
			conflicts = true
			rec.Message = "a different record exists; pass replace to overwrite it"
		case it.Plan.Outcome == dnsrecords.OutcomeUnchanged:
			rec.Message = "already in place"
		default:
			d, undo := rt.writeDomainDNS(ctx, r, app, trackDomain, st.target, it.Desired, it.Plan, domainDNSResult{Domain: trackDomain})
			rec.Message = d.Message
			if d.DNS == dnsResultError {
				failed, rec.Result = true, wwwResultFailed
				undo, rec.Message = rt.restoreAfterFailedAppend(ctx, r, app, trackDomain, st.target, it.Plan, undo, rec.Message)
			}
			out.Undo = append(out.Undo, undo...)
		}
		if skipRedirect && isWWW && rec.Result != wwwResultNotAttempted {
			rec.Message = joinNote(rec.Message, wwwNoRedirectNote)
		}
		out.Records = append(out.Records, rec)
	}
	out.Applied = !failed && !conflicts
	switch {
	case failed:
		out.Message = "a record failed; later records were not written, and the undo list covers what was"
	case conflicts:
		out.Message = "conflicting records were left alone; pass replace to overwrite them"
	}
	switch {
	case skipRedirect:
		out.RedirectMessage = "skipped: redirect was turned off"
		out.Message = joinNote(out.Message, wwwNoRedirectNote)
	case failed:
		out.RedirectMessage = "not applied because a DNS record failed"
	case conflicts:
		out.RedirectMessage = "not applied because a record was left as a conflict; resolve it and apply again"
	default:
		out.RedirectApplied, out.RedirectMessage = rt.applyWWWRedirect(ctx, r, app, domain, out.Redirect, st.res.Canonical, req.Replace)
	}
	if len(out.Undo) > 0 {
		state := "applied"
		if !out.Applied {
			state = "partial"
		}
		run := goLiveResult{App: app, Domain: domain, State: state, Steps: []goLiveStep{}, undo: out.Undo}
		rt.recordGoLiveRun(ctx, &run)
		out.RunID = run.RunID
	}
	writeJSON(w, http.StatusOK, out)
}

// restoreAfterFailedAppend puts back records a replace deleted when the new
// record was then rejected, so a failed apply does not leave the name empty.
func (rt *Router) restoreAfterFailedAppend(ctx context.Context, r *http.Request, app, domain string, t *dnsTarget, plan dnsrecords.Plan, undo []undoAction, msg string) ([]undoAction, string) {
	if len(plan.Replace) == 0 || len(undo) == 0 {
		return undo, msg
	}
	for _, u := range undo {
		if u.Kind != undoKindDNSRestore {
			// The new record may be live, so restoring could duplicate it.
			return undo, msg
		}
	}
	if _, err := t.Manager.AppendRecords(ctx, t.Zone, recordsOf(plan.Replace)); err != nil {
		rt.logger.Error("api: www apply: restore after failed append failed", slog.String("error", err.Error()), slog.String("domain", domain))
		return undo, msg + "; the old records could not be put back, undo the run to restore them"
	}
	for _, old := range plan.Replace {
		rt.recordDNSAudit(ctx, r, store.AuditActionDNSRecordCreated, app, domain, old)
	}
	return nil, msg + "; the records it replaced were restored"
}

// wwwPreviewSummary lists what a confirmed apply would do, for the 400 reply.
func wwwPreviewSummary(items []dnsrecords.ApexWWWItem) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, previewMessage(it.Plan.Outcome, it.Desired))
	}
	return strings.Join(parts, "; ")
}

// applyWWWRedirect persists the canonical 308 through the same handler the
// redirects route uses, only when the www twin is already attached.
func (rt *Router) applyWWWRedirect(ctx context.Context, r *http.Request, app, domain string, redirect *wwwPlanRedirect, canonical string, replace bool) (bool, string) {
	if redirect == nil {
		return false, "the redirect applies only in both mode"
	}
	twin, _ := trafficpolicy.Counterpart(domain)
	if owns, err := rt.appOwnsDomain(ctx, app, twin); err != nil || !owns {
		return false, twin + " is not attached to app " + app + "; attach it, then apply the redirect"
	}
	preset := trafficpolicy.PresetWWWToApex
	if canonical == dnsrecords.CanonicalWWW {
		preset = trafficpolicy.PresetApexToWWW
	}
	body, _ := json.Marshal(canonicalRequest{Preset: preset, Attach: new(bool), Replace: replace})
	sub := r.Clone(ctx)
	sub.Body = io.NopCloser(bytes.NewReader(body))
	rec := httptest.NewRecorder()
	rt.handleSetDomainCanonical(rec, sub)
	if rec.Code == http.StatusOK {
		return true, ""
	}
	var e apiError
	if json.Unmarshal(rec.Body.Bytes(), &e) == nil && e.Error != "" {
		return false, e.Error
	}
	return false, fmt.Sprintf("the redirect was not saved (status %d)", rec.Code)
}
