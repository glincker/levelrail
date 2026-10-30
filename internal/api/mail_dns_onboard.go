package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Candidate sources, in the order the panel prefers them.
const (
	mailDomainSourceInstall = "install"
	mailDomainSourceMail    = "mail"
	mailDomainSourceApp     = "app"
	mailDomainSourceZone    = "zone"
)

// mailDNSDomain is one candidate of GET /api/v1/mail/dns/domains.
type mailDNSDomain struct {
	Domain   string   `json:"domain"`
	Source   string   `json:"source"`
	Sources  []string `json:"sources"`
	Provider string   `json:"provider,omitempty"`
}

type mailDNSDomainsResponse struct {
	Domains  []mailDNSDomain `json:"domains"`
	Warnings []string        `json:"warnings,omitempty"`
}

// mergeMailDomains de-duplicates candidates by name, keeping the first source as the primary one.
func mergeMailDomains(in []mailDNSDomain) []mailDNSDomain {
	out := []mailDNSDomain{}
	idx := map[string]int{}
	for _, c := range in {
		d := dnszones.NormalizeDomain(c.Domain)
		if d == "" || mailrecords.ValidHost(d) != nil {
			continue
		}
		if i, ok := idx[d]; ok {
			if !slices.Contains(out[i].Sources, c.Source) {
				out[i].Sources = append(out[i].Sources, c.Source)
			}
			continue
		}
		idx[d] = len(out)
		out = append(out, mailDNSDomain{Domain: d, Source: c.Source, Sources: []string{c.Source}})
	}
	return out
}

// handleMailDNSDomains handles GET /api/v1/mail/dns/domains: the domains the mail DNS panel can offer.
func (rt *Router) handleMailDNSDomains(w http.ResponseWriter, r *http.Request) {
	var cands []mailDNSDomain
	var resp mailDNSDomainsResponse
	if rt.mail.demo != nil {
		cands = append(cands, mailDNSDomain{Domain: demo.DefaultDomain, Source: mailDomainSourceInstall}, mailDNSDomain{Domain: demo.DefaultDomain, Source: mailDomainSourceZone})
	} else if svc, ok, err := rt.pickMailApp(r, r.URL.Query().Get("app")); err != nil {
		rt.internalError(w, "api: mail dns domains: list services", err)
		return
	} else if ok {
		if d, _ := mailNames(svc.Env); d != "" {
			cands = append(cands, mailDNSDomain{Domain: d, Source: mailDomainSourceInstall})
		}
	}
	if rt.mail.boxes != nil {
		md, err := rt.visibleMailDomains(r, rt.mail.boxes)
		if err != nil {
			rt.internalError(w, "api: mail dns domains: mail domains", err)
			return
		}
		for _, d := range md {
			cands = append(cands, mailDNSDomain{Domain: d.Domain, Source: mailDomainSourceMail})
		}
	}
	if rt.domains != nil {
		ds, err := rt.domains.ListServiceDomains(r.Context())
		if err != nil {
			rt.internalError(w, "api: mail dns domains: app domains", err)
			return
		}
		canSee, err := rt.appVisibilityFilter(r)
		if err != nil {
			rt.internalError(w, "api: mail dns domains: visibility", err)
			return
		}
		for _, d := range ds {
			if canSee(d.ServiceName) {
				cands = append(cands, mailDNSDomain{Domain: d.Domain, Source: mailDomainSourceApp})
			}
		}
	}
	if rt.mail.demo == nil {
		zones, warn := rt.mailCandidateZones(r)
		resp.Warnings = warn
		for _, z := range zones {
			cands = append(cands, mailDNSDomain{Domain: z, Source: mailDomainSourceZone})
		}
	}
	resp.Domains = mergeMailDomains(cands)
	for i := range resp.Domains {
		if e, ok := rt.mail.nsCache.get(resp.Domains[i].Domain); ok && e.err == nil {
			resp.Domains[i].Provider = e.det.Provider
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// mailCandidateZones lists managed zones for unscoped callers only; a provider failure is a warning, not an error.
func (rt *Router) mailCandidateZones(r *http.Request) ([]string, []string) {
	if _, filtered, err := rt.callerAppVisibility(r); err != nil || filtered {
		return nil, nil
	}
	all, err := rt.zoneProviders(r.Context())
	if err != nil {
		return nil, []string{"could not resolve the connected DNS providers"}
	}
	var out, warn []string
	for _, name := range providerNames(all) {
		zones, err := all[name].ListZones(r.Context())
		if err != nil {
			warn = append(warn, "could not list zones at "+name)
			continue
		}
		for _, z := range zones {
			if !z.Private {
				out = append(out, z.Name)
			}
		}
	}
	return out, warn
}

// mailLedgerSubject is the audit subject of a successful apply: names, types and provider ids, never values.
func mailLedgerSubject(domain, provider string, applied int, recs []mailrecords.AppliedRecord) string {
	parts := make([]string, 0, len(recs))
	for _, a := range recs {
		parts = append(parts, a.Action+":"+a.Name+"/"+a.Type+"="+strings.Join(a.IDs, "|"))
	}
	return fmt.Sprintf("mail_dns domain=%s provider=%s applied=%d %s", domain, provider, applied, strings.Join(parts, ","))
}

// mailDNSAppliedEntry is one apply in GET /api/v1/mail/dns/applied.
type mailDNSAppliedEntry struct {
	AppliedAt string                      `json:"applied_at"`
	Actor     string                      `json:"actor"`
	Provider  string                      `json:"provider"`
	ZoneID    string                      `json:"zone_id"`
	Records   []mailrecords.AppliedRecord `json:"records"`
}

type mailDNSAppliedResponse struct {
	Domain  string                `json:"domain"`
	Entries []mailDNSAppliedEntry `json:"entries"`
}

// parseMailLedger reads a ledger row back; ok is false for rows of another domain or format.
func parseMailLedger(path, domain string) (mailDNSAppliedEntry, bool) {
	zoneID, subject, found := strings.Cut(strings.TrimPrefix(path, dnsZoneAuditPrefix), "#")
	if !found {
		return mailDNSAppliedEntry{}, false
	}
	f := strings.Fields(subject)
	if len(f) < 4 || f[0] != "mail_dns" || f[1] != "domain="+domain {
		return mailDNSAppliedEntry{}, false
	}
	e := mailDNSAppliedEntry{ZoneID: zoneID, Provider: strings.TrimPrefix(f[2], "provider="), Records: []mailrecords.AppliedRecord{}}
	if len(f) < 5 {
		return e, true
	}
	for _, p := range strings.Split(f[4], ",") {
		action, rest, ok1 := strings.Cut(p, ":")
		nt, ids, _ := strings.Cut(rest, "=")
		name, typ, ok2 := strings.Cut(nt, "/")
		if !ok1 || !ok2 {
			continue
		}
		a := mailrecords.AppliedRecord{Name: name, Type: typ, Action: action, IDs: []string{}}
		if ids != "" {
			a.IDs = strings.Split(ids, "|")
		}
		e.Records = append(e.Records, a)
	}
	return e, true
}

// handleMailDNSApplied handles GET /api/v1/mail/dns/applied?domain=: what mail DNS added, from the audit ledger.
func (rt *Router) handleMailDNSApplied(w http.ResponseWriter, r *http.Request) {
	domain := dnszones.NormalizeDomain(r.URL.Query().Get("domain"))
	if err := mailrecords.ValidHost(domain); err != nil {
		writeError(w, http.StatusBadRequest, "domain: "+err.Error())
		return
	}
	resp := mailDNSAppliedResponse{Domain: domain, Entries: []mailDNSAppliedEntry{}}
	if ok, err := rt.callerSeesZone(r, domain); err != nil {
		rt.internalError(w, "api: mail dns applied: resolve caller visibility", err)
		return
	} else if !ok {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	rows, err := rt.auditLog.ListAuditEntries(r.Context(), 200, nil, store.AuditEntryFilter{Action: auditMailDNSApply, Search: "domain=" + domain + " "})
	if err != nil {
		rt.internalError(w, "api: mail dns applied: list audit", err)
		return
	}
	for _, row := range rows {
		if row.StatusCode != http.StatusOK {
			continue
		}
		if e, ok := parseMailLedger(row.Path, domain); ok {
			e.AppliedAt, e.Actor = row.CreatedAt, row.ActorName
			resp.Entries = append(resp.Entries, e)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// mailDefaultProvider fills ?provider= from detection when the caller named none.
func (rt *Router) mailDefaultProvider(ctx context.Context, r *http.Request, domain string) {
	q := r.URL.Query()
	if rt.mail.demo != nil || q.Get("provider") != "" {
		return
	}
	if name := rt.detectedZoneProvider(ctx, domain); name != "" {
		q.Set("provider", name)
		r.URL.RawQuery = q.Encode()
	}
}
