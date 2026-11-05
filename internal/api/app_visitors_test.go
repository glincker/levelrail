package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func visitorsRouter(t *testing.T, log *ingress.AccessLog) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	stats := ingress.NewRequestStats()
	stats.SetHostOwners(map[string]string{"web.example.com": "web"})
	rt := NewRouter(discardLogger(), testBrand(), db, WithAccessSource(EdgeAccess{Stats: stats, Log: log}))
	seedApp(t, db, "web")
	return rt, loginTestSession(t, rt, db)
}

func getVisitors(t *testing.T, rt *Router, cookie *http.Cookie, query string) AppVisitorsResponse {
	t.Helper()
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors"+query, "")
	var res AppVisitorsResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("GET visitors%s = %d: %s", query, rec.Code, rec.Body.String())
	}
	return res
}

func TestAppVisitors_LiveRingWindows(t *testing.T) {
	t.Parallel()
	log := ingress.NewAccessLog(100)
	now := time.Now()
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-time.Minute), Host: "web.example.com", Path: "/", Status: 200, Client: netip.MustParseAddr("192.0.2.1")})
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-2 * time.Minute), Host: "web.example.com", Path: "/x", Status: 404, Client: netip.MustParseAddr("192.0.2.2")})
	rt, cookie := visitorsRouter(t, log)
	res := getVisitors(t, rt, cookie, "")
	if res.Window != "24h" || res.Requests != 2 || res.Visitors != 2 || res.Status["4xx"] != 1 {
		t.Fatalf("24h = %+v", res)
	}
	if res = getVisitors(t, rt, cookie, "?window=30d&domain=web.example.com"); res.Window != "30d" || res.Requests != 2 {
		t.Fatalf("30d = %+v", res)
	}
	if res = getVisitors(t, rt, cookie, "?domain=other.example.com"); res.Requests != 0 {
		t.Fatalf("other domain = %+v", res)
	}
}

func TestAppVisitors_RejectsUnknownWindow(t *testing.T) {
	t.Parallel()
	rt, cookie := visitorsRouter(t, ingress.NewAccessLog(10))
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors?window=90d", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAppVisitors_NotImplementedWithoutAccessLog(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	cookie := loginTestSession(t, rt, db)
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAppVisitors_HistorySurvivesRestart(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	stats := ingress.NewRequestStats()
	stats.SetHostOwners(map[string]string{"web.example.com": "web"})
	seedApp(t, db, "web")
	retention := 7 * 24 * time.Hour
	attach := func(log *ingress.AccessLog) {
		log.SetVisitorHistory(db, stats.OwnerOf, func() time.Duration { return retention }, slog.Default())
	}

	first := ingress.NewAccessLog(100)
	attach(first)
	old := time.Now().Add(-3 * 24 * time.Hour)
	for i, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.1"} {
		first.Record("web.example.com", ingress.AccessRecord{Time: old.Add(time.Duration(i) * time.Second), Host: "web.example.com", Path: "/old", Status: 200, Referrer: "news.test", Client: netip.MustParseAddr(ip)})
	}
	if err := first.FlushVisitorHistory(t.Context()); err != nil {
		t.Fatal(err)
	}

	// A fresh log stands in for the restarted process: its ring is empty.
	second := ingress.NewAccessLog(100)
	attach(second)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAccessSource(EdgeAccess{Stats: stats, Log: second}))
	cookie := loginTestSession(t, rt, db)
	if res := getVisitors(t, rt, cookie, "?window=24h"); res.Requests != 0 {
		t.Fatalf("24h must not include 3 day old traffic: %+v", res)
	}
	res := getVisitors(t, rt, cookie, "?window=7d")
	if res.Requests != 3 || res.Visitors != 2 || res.Status["2xx"] != 3 || len(res.TopPaths) != 1 || res.TopPaths[0].Label != "/old" || len(res.TopRefs) != 1 {
		t.Fatalf("7d after restart = %+v", res)
	}
	if res.OldestKept.IsZero() || res.OldestKept.After(old) {
		t.Fatalf("oldest_record = %v, want at or before %v", res.OldestKept, old)
	}

	retention = 0
	if err := second.FlushVisitorHistory(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.ListVisitorRollups(t.Context(), "web", "", time.Now().Add(-30*24*time.Hour), time.Now().Add(time.Hour)); err != nil || len(rows) != 0 {
		t.Fatalf("rows after disabling = %d, %v", len(rows), err)
	}
	if res = getVisitors(t, rt, cookie, "?window=7d"); res.Requests != 0 {
		t.Fatalf("disabled history still served: %+v", res)
	}
}

var _ ingress.VisitorHistoryStore = (*store.DB)(nil)

func TestCSVSafeField(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"=1+1": "'=1+1", "+x": "'+x", "-x": "'-x", "@s": "'@s", "\tx": "'\tx", "\rx": "'\rx", "/ok": "/ok", "": ""} {
		if got := csvSafeField(in); got != want {
			t.Errorf("csvSafeField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteVisitorsCSVEscapes(t *testing.T) {
	t.Parallel()
	var res AppVisitorsResponse
	res.Window = "24h"
	res.TopPaths = []ingress.CountedItem{{Label: `/a,"b"`, Count: 2}, {Label: "=cmd", Count: 1}}
	var buf bytes.Buffer
	if err := writeVisitorsCSV(&buf, res); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "section,key,requests,visitors,extra\n") || !strings.Contains(out, "path,\"/a,\"\"b\"\"\",2,0,\n") || !strings.Contains(out, "path,'=cmd,1,0,\n") {
		t.Fatalf("csv = %q", out)
	}
}

func TestAppVisitorsExportCSV(t *testing.T) {
	t.Parallel()
	log := ingress.NewAccessLog(100)
	now := time.Now()
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-time.Minute), Host: "web.example.com", Path: "/", Status: 200, Client: netip.MustParseAddr("192.0.2.1")})
	rt, cookie := visitorsRouter(t, log)
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors/export?window=7d", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="visitors-web-7d.csv"` {
		t.Fatalf("status %d headers %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "summary,total,1,1,window=7d\n") || !strings.Contains(body, "status,2xx,1,0,\n") || !strings.Contains(body, "path,/,1,0,\n") {
		t.Fatalf("body = %q", body)
	}
	if rec = doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors/export?window=90d", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad window = %d", rec.Code)
	}
}

func TestAppVisitorsExportNotImplementedWithoutAccessLog(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	cookie := loginTestSession(t, rt, db)
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/visitors/export", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d", rec.Code)
	}
}
