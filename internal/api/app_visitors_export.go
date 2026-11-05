package api

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// visitorCSVHeader is the fixed long-format column set; the web export button relies on it.
var visitorCSVHeader = []string{"section", "key", "requests", "visitors", "extra"}

// csvSafeField neutralises spreadsheet formula injection by prefixing risky leading characters.
func csvSafeField(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// writeVisitorsCSV renders the summary as one long-format CSV. encoding/csv handles RFC 4180 quoting.
func writeVisitorsCSV(w io.Writer, res AppVisitorsResponse) error {
	cw := csv.NewWriter(w)
	row := func(section, key string, requests, visitors int, extra string) {
		_ = cw.Write([]string{section, csvSafeField(key), strconv.Itoa(requests), strconv.Itoa(visitors), csvSafeField(extra)})
	}
	blank := func(section, key, extra string) {
		_ = cw.Write([]string{section, csvSafeField(key), "", "", csvSafeField(extra)})
	}
	_ = cw.Write(visitorCSVHeader)
	row("summary", "total", res.Requests, res.Visitors, "window="+res.Window)
	if !res.OldestKept.IsZero() {
		blank("summary", "oldest_record", res.OldestKept.UTC().Format(time.RFC3339))
	}
	classes := make([]string, 0, len(res.Status))
	for k := range res.Status {
		classes = append(classes, k)
	}
	sort.Strings(classes)
	for _, k := range classes {
		row("status", k, res.Status[k], 0, "")
	}
	for _, b := range res.Series {
		row("series", b.Start.UTC().Format(time.RFC3339), b.Requests, b.Visitors, "")
	}
	for _, p := range res.TopPaths {
		row("path", p.Label, p.Count, 0, "")
	}
	for _, p := range res.TopRefs {
		row("referrer", p.Label, p.Count, 0, "")
	}
	for _, d := range res.Domains {
		row("domain", d.Domain, d.Requests, d.Visitors, "")
	}
	cw.Flush()
	return cw.Error()
}

// handleAppVisitorsExport serves the same summary as handleAppVisitors as a CSV download.
func (rt *Router) handleAppVisitorsExport(w http.ResponseWriter, r *http.Request) {
	if f := r.URL.Query().Get("format"); f != "" && f != "csv" {
		writeError(w, http.StatusBadRequest, "format must be csv")
		return
	}
	res, ok := rt.visitorSummary(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="visitors-%s-%s.csv"`, safeFilePart(res.App), res.Window))
	w.Header().Set("Cache-Control", "no-store")
	_ = writeVisitorsCSV(w, res)
}

// safeFilePart keeps header values to a conservative character set.
func safeFilePart(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, s)
}
