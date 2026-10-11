package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/miekg/dns"
)

// decodeJSONBodyLimit is decodeJSONBody with a caller chosen size cap, for
// zone file imports that outgrow the default.
func decodeJSONBodyLimit(r *http.Request, dst any, limit int64) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit)).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// handleListDNSTemplates handles GET /api/v1/dns/templates.
func (rt *Router) handleListDNSTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dnszones.Templates)
}

type applyDNSTemplateRequest struct {
	Params map[string]string `json:"params"`
	Apply  bool              `json:"apply,omitempty"`
}

// handleApplyDNSTemplate handles POST /api/v1/dns/zones/{zone}/templates/{id}:
// a plan by default, applied when apply is true. TXT values already at the
// same name are kept, so verification tokens survive an SPF template.
func (rt *Router) handleApplyDNSTemplate(w http.ResponseWriter, r *http.Request) {
	var req applyDNSTemplateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tpl, found := dnszones.FindTemplate(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "unknown template")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	rendered, err := tpl.Render(req.Params)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	var sets []dnszones.RecordSet
	var errs []string
	for _, s := range dnszones.MergeTemplate(existing, rendered) {
		n, nerr := dnszones.Normalize(s, z.Name, dnsDefaultTTL())
		if nerr != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", s.Name, s.Type, nerr))
			continue
		}
		sets = append(sets, n)
	}
	resp := importDNSRecordsResponse{Plan: dnszones.BuildPlan(existing, sets, false, p.Capabilities()), Errors: errs}
	if !req.Apply {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(errs) > 0 || resp.Plan.Blocked {
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	n, err := dnszones.ApplyPlan(r.Context(), p, z, resp.Plan)
	resp.Applied = n
	rt.auditDNS(r, auditDNSRecordTemplate, z, tpl.ID, statusFor(err))
	if err != nil {
		rt.dnsProviderError(w, "apply template", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type discoverResponse struct {
	Servers []string             `json:"servers"`
	Records []dnszones.RecordSet `json:"records"`
	Plan    dnszones.Plan        `json:"plan"`
}

// handleDiscoverDNSZoneRecords handles GET /api/v1/dns/zones/{zone}/discover?names=a,b:
// resolves common names at the domain's current name servers (AXFR is
// almost always refused), as a preview the operator imports through POST .../import.
func (rt *Router) handleDiscoverDNSZoneRecords(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	full, err := p.GetZone(r.Context(), z.ID)
	if err != nil {
		rt.dnsProviderError(w, "get zone", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*domainCheckLookupTimeout)
	defer cancel()
	q := rt.querier()
	var servers []string
	for _, res := range dnsCheckServers() {
		a := q.Query(ctx, res, full.Name, dns.TypeNS)
		for _, ns := range a.Values {
			if !slices.Contains(full.NameServers, ns) && !slices.Contains(servers, ns) {
				servers = append(servers, ns)
			}
		}
	}
	if len(servers) == 0 {
		servers = slices.DeleteFunc(slices.Clone(dnsCheckServers()), func(s string) bool { return s == dnszones.ServerSystem })
	}
	var extra []string
	for _, n := range strings.Split(r.URL.Query().Get("names"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			extra = append(extra, n)
		}
	}
	found := dnszones.Discover(ctx, q, full.Name, extra, servers)
	sets := make([]dnszones.RecordSet, 0, len(found))
	for _, s := range found {
		if n, nerr := dnszones.Normalize(s, full.Name, dnsDefaultTTL()); nerr == nil {
			sets = append(sets, n)
		}
	}
	existing, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	writeJSON(w, http.StatusOK, discoverResponse{Servers: servers, Records: sets, Plan: dnszones.BuildPlan(existing, sets, false, p.Capabilities())})
}

// handleDNSCheck handles GET /api/v1/dns/check?name=&type=&zone=: the answer
// for one name from the system resolver, public resolvers and, when the zone
// is known, its own name servers, with TTLs and agreement.
func (rt *Router) handleDNSCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := dnszones.NormalizeDomain(q.Get("name"))
	typ := strings.ToUpper(q.Get("type"))
	if typ == "" {
		typ = "A"
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := dnszones.QType(typ); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var auth, expected []string
	if all, err := rt.zoneProviders(r.Context()); err == nil {
		for _, pn := range providerNames(all) {
			if a, e, found := zoneAnswer(r.Context(), all[pn], name, typ, q.Get("zone")); found {
				auth, expected = a, e
				break
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*domainCheckLookupTimeout)
	defer cancel()
	prop, err := dnszones.CheckPropagation(ctx, rt.querier(), name, typ, dnsCheckServers(), auth, expected)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prop)
}

// zoneAnswer finds the longest zone containing name and what it holds for typ.
func zoneAnswer(ctx context.Context, p dnszones.Provider, name, typ, ref string) ([]string, []string, bool) {
	zones, err := p.ListZones(ctx)
	if err != nil {
		return nil, nil, false
	}
	var best dnszones.Zone
	for _, z := range zones {
		match := name == z.Name || strings.HasSuffix(name, "."+z.Name)
		if ref != "" {
			match = match && (z.ID == ref || z.Name == dnszones.NormalizeDomain(ref))
		}
		if match && len(z.Name) > len(best.Name) {
			best = z
		}
	}
	if best.ID == "" {
		return nil, nil, false
	}
	var expected []string
	if sets, err := p.ListRecordSets(ctx, best); err == nil {
		rel := dnszones.RelativeName(name, best.Name)
		for _, s := range sets {
			if s.Name == rel && s.Type == typ && s.SetIdentifier == "" && !s.Proxied {
				expected = s.Values
			}
		}
	}
	return best.NameServers, expected, true
}

// requireHealthChecker resolves a provider with health checks (Route53).
func (rt *Router) requireHealthChecker(w http.ResponseWriter, r *http.Request) (dnszones.HealthChecker, bool) {
	all, err := rt.zoneProviders(r.Context())
	if err != nil {
		rt.internalError(w, "api: resolve dns zone providers failed", err)
		return nil, false
	}
	for _, p := range all {
		if hc, ok := p.(dnszones.HealthChecker); ok {
			return hc, true
		}
	}
	writeError(w, http.StatusNotImplemented, "health checks need Route53: connect it under Domains, DNS provider")
	return nil, false
}

// handleListDNSHealthChecks handles GET /api/v1/dns/health-checks.
func (rt *Router) handleListDNSHealthChecks(w http.ResponseWriter, r *http.Request) {
	hc, ok := rt.requireHealthChecker(w, r)
	if !ok {
		return
	}
	list, err := hc.ListHealthChecks(r.Context())
	if err != nil {
		rt.dnsProviderError(w, "list health checks", err)
		return
	}
	if list == nil {
		list = []dnszones.HealthCheck{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleCreateDNSHealthCheck handles POST /api/v1/dns/health-checks.
func (rt *Router) handleCreateDNSHealthCheck(w http.ResponseWriter, r *http.Request) {
	var req dnszones.HealthCheck
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Type = strings.ToUpper(req.Type)
	if err := dnszones.ValidateHealthCheck(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hc, ok := rt.requireHealthChecker(w, r)
	if !ok {
		return
	}
	out, err := hc.CreateHealthCheck(r.Context(), req)
	if err != nil {
		rt.dnsProviderError(w, "create health check", err)
		return
	}
	rt.auditDNS(r, auditDNSHealthCreate, dnszones.Zone{ID: "health-checks"}, out.ID, http.StatusCreated)
	writeJSON(w, http.StatusCreated, out)
}

// handleDeleteDNSHealthCheck handles DELETE /api/v1/dns/health-checks/{id}.
func (rt *Router) handleDeleteDNSHealthCheck(w http.ResponseWriter, r *http.Request) {
	hc, ok := rt.requireHealthChecker(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := hc.DeleteHealthCheck(r.Context(), id); err != nil {
		rt.dnsProviderError(w, "delete health check", err)
		return
	}
	rt.auditDNS(r, auditDNSHealthDelete, dnszones.Zone{ID: "health-checks"}, id, http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
