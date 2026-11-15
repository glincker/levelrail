package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

func egressReportTestLog(privacy string) *network.QueryLog {
	l := network.NewQueryLog(network.QueryLogConfig{
		Enabled: func() bool { return true }, Size: func() int { return 50 },
		Privacy: func() string { return privacy },
		AppForIP: func(a netip.Addr) string {
			return map[string]string{"172.18.0.5": "web", "172.18.0.6": "other"}[a.String()]
		},
	})
	now := time.Now()
	web, other := netip.MustParseAddr("172.18.0.5"), netip.MustParseAddr("172.18.0.6")
	obs := func(src netip.Addr, name, rcode, reason string) {
		l.Observe(src, name, "A", rcode, reason, 0, time.Microsecond, now)
	}
	obs(web, "api.example.com.", "REFUSED", network.ReasonRefused)
	obs(web, "api.example.com.", "REFUSED", network.ReasonRefused)
	obs(web, "cdn.static.example.org.", "REFUSED", network.ReasonRefused)
	obs(web, "tracker.bad.test.", "REFUSED", network.ReasonRefused)
	obs(web, "db.acme.", "NOERROR", network.ReasonLocalRecord)
	obs(other, "other-only.test.", "REFUSED", network.ReasonRefused)
	return l
}

func TestAppEgressReportListsDeniedNames(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	rt.SetDNSQueryLog(egressReportTestLog(network.QueryPrivacyNamesOnly))
	if err := db.UpdateServiceEgressPolicy(context.Background(), "web", &store.ServiceEgressPolicy{
		Mode: store.EgressModeAllowlist, Allow: []store.ServiceEgressAllow{{Host: "api.example.com", Port: 443}, {Host: "*.example.org", Port: 443}},
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/egress-report", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out egressReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Names) != 3 || out.Names[0].Name != "tracker.bad.test" || !out.Names[0].WouldDeny {
		t.Fatalf("denied name should sort first: %+v", out.Names)
	}
	for _, n := range out.Names[1:] {
		if n.WouldDeny {
			t.Fatalf("%s matched the allowlist: %+v", n.Name, out.Names)
		}
		if n.Name == "api.example.com" && n.Count != 2 {
			t.Fatalf("count = %d", n.Count)
		}
	}
}

func TestAppEgressReportNoDataCases(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	rt.SetDNSQueryLog(egressReportTestLog(network.QueryPrivacyCountersOnly))
	get := func(path string) (int, egressReportResponse) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		var out egressReportResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, out := get("/api/v1/apps/web/egress-report"); code != http.StatusOK || out.Message == "" || len(out.Names) != 0 {
		t.Fatalf("counters_only: %d %+v", code, out)
	}
	if code, _ := get("/api/v1/apps/missing/egress-report"); code != http.StatusNotFound {
		t.Fatalf("missing app = %d", code)
	}
}

func TestAppEgressReportRequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodGet, "/api/v1/apps/web/egress-report"}})
}

func TestBuildEgressReportTruncates(t *testing.T) {
	now := time.Now()
	recs := []network.QueryRecord{
		{QName: "a.test.", Reason: network.ReasonRefused, Time: now},
		{QName: "b.test.", Reason: network.ReasonRefused, Time: now},
	}
	rows, cut := buildEgressReport(recs, []string{"a.test"}, 1)
	if len(rows) != 1 || cut != 1 || rows[0].Name != "b.test" {
		t.Fatalf("rows %+v cut %d", rows, cut)
	}
}
