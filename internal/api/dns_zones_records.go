package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

type dnsRecordView struct {
	dnszones.RecordSet
	Managed bool `json:"managed,omitempty"`
}

type dnsRecordsListResponse struct {
	Zone         dnszones.Zone         `json:"zone"`
	Provider     string                `json:"provider"`
	Capabilities dnszones.Capabilities `json:"capabilities"`
	Records      []dnsRecordView       `json:"records"`
	Total        int                   `json:"total"`
}

func filterSets(sets []dnszones.RecordSet, typ, q string) []dnszones.RecordSet {
	typ, q = strings.ToUpper(strings.TrimSpace(typ)), strings.ToLower(strings.TrimSpace(q))
	return slices.DeleteFunc(sets, func(s dnszones.RecordSet) bool {
		if typ != "" && s.Type != typ {
			return true
		}
		if q == "" {
			return false
		}
		hay := strings.ToLower(s.Name + " " + strings.Join(s.Values, " ") + " " + s.SetIdentifier)
		return !strings.Contains(hay, q)
	})
}

// handleListDNSZoneRecords handles GET /api/v1/dns/zones/{zone}/records?type=&q=.
func (rt *Router) handleListDNSZoneRecords(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	sets, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	total := len(sets)
	sets = filterSets(sets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	views := make([]dnsRecordView, 0, len(sets))
	for _, s := range sets {
		views = append(views, dnsRecordView{RecordSet: s, Managed: dnszones.IsManagedApexType(s) || s.Alias != nil})
	}
	writeJSON(w, http.StatusOK, dnsRecordsListResponse{Zone: z, Provider: p.Name(), Capabilities: p.Capabilities(), Records: views, Total: total})
}

type dnsRecordWriteResponse struct {
	Record dnszones.RecordSet `json:"record"`
	Issues []dnszones.Issue   `json:"issues,omitempty"`
}

type dnsRecordConflictResponse struct {
	Error  string           `json:"error"`
	Issues []dnszones.Issue `json:"issues"`
}

// prepareRecord normalizes rs and checks provider support and conflicts.
// It writes the error response itself when the set cannot be written.
func (rt *Router) prepareRecord(w http.ResponseWriter, r *http.Request, p dnszones.Provider, z dnszones.Zone, rs dnszones.RecordSet, ignore *dnszones.Key) (dnszones.RecordSet, []dnszones.RecordSet, []dnszones.Issue, bool) {
	norm, err := dnszones.Normalize(rs, z.Name, dnsDefaultTTL())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return norm, nil, nil, false
	}
	if err := dnszones.CheckCapabilities(norm, p.Capabilities()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return norm, nil, nil, false
	}
	existing, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return norm, nil, nil, false
	}
	issues := dnszones.Conflicts(existing, norm, p.Capabilities(), ignore)
	if dnszones.HasErrors(issues) {
		writeJSON(w, http.StatusConflict, dnsRecordConflictResponse{Error: "record conflicts with the zone", Issues: issues})
		return norm, existing, issues, false
	}
	return norm, existing, issues, true
}

// handleCreateDNSZoneRecord handles POST /api/v1/dns/zones/{zone}/records.
func (rt *Router) handleCreateDNSZoneRecord(w http.ResponseWriter, r *http.Request) {
	var req dnszones.RecordSet
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	rs, _, issues, ok := rt.prepareRecord(w, r, p, z, req, nil)
	if !ok {
		return
	}
	if err := p.UpsertRecordSet(r.Context(), z, rs); err != nil {
		rt.dnsProviderError(w, "create record", err)
		return
	}
	rt.auditDNS(r, auditDNSRecordCreate, z, rs.Name+"/"+rs.Type, http.StatusCreated)
	writeJSON(w, http.StatusCreated, dnsRecordWriteResponse{Record: rs, Issues: issues})
}

type updateDNSZoneRecordRequest struct {
	Original dnszones.Key       `json:"original"`
	Record   dnszones.RecordSet `json:"record"`
}

// handleUpdateDNSZoneRecord handles PUT /api/v1/dns/zones/{zone}/records. A
// renamed set is written under its new key before the old one is removed.
func (rt *Router) handleUpdateDNSZoneRecord(w http.ResponseWriter, r *http.Request) {
	var req updateDNSZoneRecordRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	orig := dnszones.Key{Name: dnszones.RelativeName(req.Original.Name, z.Name), Type: strings.ToUpper(req.Original.Type), SetIdentifier: req.Original.SetIdentifier}
	rs, existing, issues, ok := rt.prepareRecord(w, r, p, z, req.Record, &orig)
	if !ok {
		return
	}
	i := slices.IndexFunc(existing, func(e dnszones.RecordSet) bool { return e.Key() == orig })
	if i < 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("record set %s %s not found", orig.Name, orig.Type))
		return
	}
	if dnszones.IsManagedApexType(existing[i]) || existing[i].Alias != nil {
		writeError(w, http.StatusBadRequest, "this record set is managed by the provider")
		return
	}
	if err := p.UpsertRecordSet(r.Context(), z, rs); err != nil {
		rt.dnsProviderError(w, "update record", err)
		return
	}
	if rs.Key() != orig {
		if err := p.DeleteRecordSet(r.Context(), z, orig); err != nil {
			rt.dnsProviderError(w, "remove renamed record", err)
			return
		}
	}
	rt.auditDNS(r, auditDNSRecordUpdate, z, orig.Name+"/"+orig.Type+" -> "+rs.Name+"/"+rs.Type, http.StatusOK)
	writeJSON(w, http.StatusOK, dnsRecordWriteResponse{Record: rs, Issues: issues})
}

// handleDeleteDNSZoneRecord handles DELETE
// /api/v1/dns/zones/{zone}/records?name=&type=&set_identifier=.
func (rt *Router) handleDeleteDNSZoneRecord(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("name") == "" || q.Get("type") == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	k := dnszones.Key{Name: dnszones.RelativeName(q.Get("name"), z.Name), Type: strings.ToUpper(q.Get("type")), SetIdentifier: q.Get("set_identifier")}
	if dnszones.IsManagedApexType(dnszones.RecordSet{Name: k.Name, Type: k.Type}) {
		writeError(w, http.StatusBadRequest, "apex NS and SOA are managed by the provider")
		return
	}
	if err := p.DeleteRecordSet(r.Context(), z, k); err != nil {
		rt.dnsProviderError(w, "delete record", err)
		return
	}
	rt.auditDNS(r, auditDNSRecordDelete, z, k.Name+"/"+k.Type, http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": k})
}

type importDNSRecordsRequest struct {
	Format  string               `json:"format"`
	Content string               `json:"content,omitempty"`
	Records []dnszones.RecordSet `json:"records,omitempty"`
	Replace bool                 `json:"replace,omitempty"`
	Apply   bool                 `json:"apply,omitempty"`
	Confirm string               `json:"confirm,omitempty"`
}

type importDNSRecordsResponse struct {
	Plan     dnszones.Plan `json:"plan"`
	Warnings []string      `json:"warnings,omitempty"`
	Errors   []string      `json:"errors,omitempty"`
	Applied  int           `json:"applied"`
}

// parseImport turns the request into normalized sets plus per set errors.
func parseImport(req importDNSRecordsRequest, zone string) ([]dnszones.RecordSet, []string, []string, error) {
	var (
		sets     []dnszones.RecordSet
		warnings []string
		err      error
	)
	switch strings.ToLower(req.Format) {
	case dnszones.FormatBIND:
		sets, warnings, err = dnszones.ParseBIND(req.Content, zone, dnsDefaultTTL())
	case dnszones.FormatJSON, "":
		sets = req.Records
		if req.Content != "" {
			sets, err = dnszones.ParseJSON([]byte(req.Content))
		}
	default:
		return nil, nil, nil, errors.New("format must be json or bind")
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var errs []string
	out := make([]dnszones.RecordSet, 0, len(sets))
	for _, s := range sets {
		n, nerr := dnszones.Normalize(s, zone, dnsDefaultTTL())
		if nerr != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", s.Name, s.Type, nerr))
			continue
		}
		out = append(out, n)
	}
	return out, warnings, errs, nil
}

// handleImportDNSZoneRecords handles POST /api/v1/dns/zones/{zone}/records/import.
// It always returns the plan; apply executes it only when nothing is blocked.
// Replace deletes sets missing from the import, so it needs the zone name typed.
func (rt *Router) handleImportDNSZoneRecords(w http.ResponseWriter, r *http.Request) {
	var req importDNSRecordsRequest
	if err := decodeJSONBodyLimit(r, &req, 4<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	sets, warnings, errs, err := parseImport(req, z.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	resp := importDNSRecordsResponse{Plan: dnszones.BuildPlan(existing, sets, req.Replace, p.Capabilities()), Warnings: warnings, Errors: errs}
	if !req.Apply {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	switch {
	case len(errs) > 0 || resp.Plan.Blocked:
		writeJSON(w, http.StatusConflict, resp)
		return
	case req.Replace && dnszones.NormalizeDomain(req.Confirm) != z.Name:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("replace deletes records; type the zone name %q in confirm", z.Name))
		return
	}
	n, err := dnszones.ApplyPlan(r.Context(), p, z, resp.Plan)
	resp.Applied = n
	rt.auditDNS(r, auditDNSRecordImport, z, fmt.Sprintf("applied=%d replace=%t", n, req.Replace), statusFor(err))
	if err != nil {
		rt.dnsProviderError(w, fmt.Sprintf("import (after %d changes)", n), err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func statusFor(err error) int {
	if err != nil {
		return http.StatusBadGateway
	}
	return http.StatusOK
}

// handleExportDNSZoneRecords handles GET /api/v1/dns/zones/{zone}/records/export?format=json|bind.
func (rt *Router) handleExportDNSZoneRecords(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	sets, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	sets = slices.DeleteFunc(sets, dnszones.IsManagedApexType)
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == dnszones.FormatBIND {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", z.Name+".zone"))
		_, _ = w.Write([]byte(dnszones.ExportBIND(z.Name, sets)))
		return
	}
	b, err := dnszones.ExportJSON(z.Name, sets)
	if err != nil {
		rt.internalError(w, "api: export dns records failed", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", z.Name+".json"))
	_, _ = w.Write(b)
}
