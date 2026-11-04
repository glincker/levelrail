package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

type wwwApplyRequest struct {
	Mode      string `json:"mode"`
	Canonical string `json:"canonical"`
	Replace   bool   `json:"replace"`
	Confirm   bool   `json:"confirm"`
}

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
	out := wwwApplyResponse{Domain: domain, Provider: st.res.Provider, Zone: st.res.Zone, Mode: st.res.Mode,
		Canonical: st.res.Canonical, Records: []wwwApplyRecord{}, Redirect: st.res.Redirect, Undo: []undoAction{}}
	failed, conflicts := false, false
	for i, it := range st.items {
		rec := wwwApplyRecord{wwwPlanRecord: st.res.Records[i], Result: it.Plan.Outcome}
		switch {
		case failed:
			rec.Result = wwwResultNotAttempted
		case it.Plan.Outcome == dnsrecords.OutcomeConflict:
			conflicts = true
			rec.Message = "a different record exists; pass replace to overwrite it"
		case it.Plan.Outcome == dnsrecords.OutcomeUnchanged:
			rec.Message = "already in place"
		default:
			d, undo := rt.writeDomainDNS(ctx, r, app, domain, st.target, it.Desired, it.Plan, domainDNSResult{Domain: domain})
			out.Undo = append(out.Undo, undo...)
			rec.Message = d.Message
			if d.DNS == dnsResultError {
				failed, rec.Result = true, wwwResultFailed
			}
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
	if failed {
		out.RedirectMessage = "not applied because a DNS record failed"
	} else {
		out.RedirectApplied, out.RedirectMessage = rt.applyWWWRedirect(ctx, r, app, domain, out.Redirect, st.res.Canonical, req.Replace)
	}
	writeJSON(w, http.StatusOK, out)
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
