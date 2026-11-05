package api

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
)

// domainsCSVHeader is the fixed column set; kind is app, standalone or parked.
var domainsCSVHeader = []string{"domain", "kind", "service", "target", "environment", "parked_from", "parked_at", "note", "tls_owner", "tls_days_left"}

// writeDomainsCSV renders live and parked domains as one CSV. Only names and state go out, never settings or credentials.
func writeDomainsCSV(w io.Writer, live []domainResource, parked []parkedDomainResource) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(domainsCSVHeader)
	for _, d := range live {
		days := ""
		if d.TLSDaysLeft != nil {
			days = strconv.Itoa(*d.TLSDaysLeft)
		}
		_ = cw.Write([]string{csvSafeField(d.Domain), csvSafeField(d.Kind), csvSafeField(d.ServiceName), csvSafeField(d.Target), csvSafeField(d.Environment), "", "", "", csvSafeField(d.TLSOwner), days})
	}
	for _, p := range parked {
		_ = cw.Write([]string{csvSafeField(p.Domain), "parked", "", "", "", csvSafeField(p.ParkedFrom), csvSafeField(p.ParkedAt), csvSafeField(p.Note), "", ""})
	}
	cw.Flush()
	return cw.Error()
}

// collectPages runs a list handler page by page, so the export reuses its authz filtering and stays under the page cap.
func collectPages[T any](r *http.Request, h http.HandlerFunc) ([]T, int) {
	var all []T
	for offset := 0; ; offset += appListMaxLimit {
		q := url.Values{"limit": {strconv.Itoa(appListMaxLimit)}, "offset": {strconv.Itoa(offset)}}
		req := r.Clone(r.Context())
		req.URL.RawQuery = q.Encode()
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusOK {
			return all, rec.Code
		}
		var page []T
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			return all, http.StatusInternalServerError
		}
		all = append(all, page...)
		if len(page) < appListMaxLimit {
			return all, http.StatusOK
		}
	}
}

// handleDomainsExport handles GET /api/v1/domains/export: domains and parked domains as a CSV download.
func (rt *Router) handleDomainsExport(w http.ResponseWriter, r *http.Request) {
	if f := r.URL.Query().Get("format"); f != "" && f != "csv" {
		writeError(w, http.StatusBadRequest, "format must be csv")
		return
	}
	live, code := collectPages[domainResource](r, rt.handleListDomains)
	if code != http.StatusOK {
		writeError(w, code, "could not list domains")
		return
	}
	// A store without parked support just exports the live domains.
	parked, code := collectPages[parkedDomainResource](r, rt.handleListParkedDomains)
	if code != http.StatusOK && code != http.StatusNotImplemented {
		writeError(w, code, "could not list parked domains")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="domains.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	_ = writeDomainsCSV(w, live, parked)
}
