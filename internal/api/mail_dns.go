package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

const auditMailDNSApply = "mail_dns.apply"

type mailDNSPlanRequest struct {
	mailrecords.Spec
	ReplaceExisting []string `json:"replace_existing,omitempty"`
}

type mailDNSApplyRequest struct {
	mailDNSPlanRequest
	Confirm  bool   `json:"confirm"`
	PlanHash string `json:"plan_hash"`
}

type mailDNSApplyResponse struct {
	Applied int                         `json:"applied"`
	Plan    mailrecords.PlanResult      `json:"plan"`
	Records []mailrecords.AppliedRecord `json:"records"`
}

// mailDNSTarget decodes the spec and resolves its zone, writing the error response itself.
func (rt *Router) mailDNSTarget(w http.ResponseWriter, r *http.Request, req *mailDNSPlanRequest) (dnszones.Provider, dnszones.Zone, mailrecords.PlanOptions, bool) {
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	rt.mailDemoFillSpec(&req.Spec)
	if _, _, err := mailrecords.Build(req.Spec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, dnszones.Zone{}, mailrecords.PlanOptions{}, false
	}
	replace, err := mailrecords.NormalizeReplace(req.ReplaceExisting)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, dnszones.Zone{}, mailrecords.PlanOptions{}, false
	}
	rt.mailDefaultProvider(r.Context(), r, req.Domain)
	r.SetPathValue("zone", req.Domain)
	p, z, ok := rt.mailZone(w, r)
	return p, z, mailrecords.PlanOptions{ReplaceExisting: replace}, ok
}

// handleMailDNSPlan handles POST /api/v1/mail/dns/plan: a dry run that diffs the mail records against the live zone.
func (rt *Router) handleMailDNSPlan(w http.ResponseWriter, r *http.Request) {
	var req mailDNSPlanRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, z, opts, ok := rt.mailDNSTarget(w, r, &req)
	if !ok {
		return
	}
	res, err := mailrecords.Plan(r.Context(), p, z, req.Spec, opts)
	if err != nil {
		rt.dnsProviderError(w, "mail dns plan", err)
		return
	}
	if len(res.Shadowed) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": res.Shadowed[0], "plan": res})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleMailDNSApply handles POST /api/v1/mail/dns/apply: writes a confirmed plan, refusing a stale one.
func (rt *Router) handleMailDNSApply(w http.ResponseWriter, r *http.Request) {
	var req mailDNSApplyRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Confirm || req.PlanHash == "" {
		writeError(w, http.StatusBadRequest, "confirm must be true and plan_hash must come from a plan response")
		return
	}
	p, z, opts, ok := rt.mailDNSTarget(w, r, &req.mailDNSPlanRequest)
	if !ok {
		return
	}
	n, res, err := mailrecords.Apply(r.Context(), p, z, req.Spec, opts, req.PlanHash)
	switch {
	case errors.Is(err, mailrecords.ErrStalePlan), errors.Is(err, mailrecords.ErrBlocked):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "plan": res})
		return
	case err != nil:
		rt.auditDNS(r, auditMailDNSApply, z, res.AuditSubject(n), http.StatusBadGateway)
		rt.dnsProviderError(w, "mail dns apply", err)
		return
	}
	recs := mailrecords.AppliedRecords(r.Context(), p, z, res)
	rt.auditDNS(r, auditMailDNSApply, z, mailLedgerSubject(req.Domain, p.Name(), n, recs), http.StatusOK)
	writeJSON(w, http.StatusOK, mailDNSApplyResponse{Applied: n, Plan: res, Records: recs})
}

// handleMailDNSCheck handles GET /api/v1/mail/dns/check?domain=: grades the live mail DNS.
func (rt *Router) handleMailDNSCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, cancel := context.WithTimeout(r.Context(), 5*domainCheckLookupTimeout)
	defer cancel()
	domain := strings.ToLower(strings.TrimSpace(q.Get("domain")))
	exp := mailrecords.Expect{MailHost: strings.ToLower(strings.TrimSpace(q.Get("mail_host"))), DKIMSelector: q.Get("dkim_selector")}
	domain = rt.mailDemoCheckArgs(domain, &exp)
	rep, err := mailrecords.Check(ctx, rt.mailResolver(), domain, exp)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := mailDNSCheckResponse{Report: rep, Status: dnsstatus.Status{ProviderSource: dnsstatus.SourcePublicDNS, Records: []dnsstatus.Record{}}}
	spec, err := mailSpecFromQuery(q)
	spec.Domain, spec.MailHost, spec.DKIMSelector = domain, exp.MailHost, exp.DKIMSelector
	if err == nil {
		rt.mailDemoFillSpec(&spec)
		if desired, _, err := mailrecords.Build(spec); err == nil {
			resp.Status = rt.mailDNSStatus(r, spec.Domain, desired)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) mailResolver() mailrecords.Resolver {
	if d := rt.mail.demo; d != nil {
		return d.zone
	}
	return net.DefaultResolver
}
