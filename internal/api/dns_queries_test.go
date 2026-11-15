package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

func dnsQueriesTestRouter(enabled bool) *Router {
	l := network.NewQueryLog(network.QueryLogConfig{
		Enabled: func() bool { return enabled }, Size: func() int { return 10 },
		Privacy:  func() string { return network.QueryPrivacyNamesOnly },
		AppForIP: func(netip.Addr) string { return "web" },
	})
	src := netip.MustParseAddr("172.18.0.5")
	l.Observe(src, "db.acme.", "A", "NOERROR", network.ReasonLocalRecord, 1, time.Microsecond, time.Now())
	l.Observe(src, "nope.acme.", "A", "NXDOMAIN", network.ReasonNoSuchApp, 0, time.Microsecond, time.Now())
	return &Router{dnsQueryLog: l}
}

func TestDNSQueriesHandlerFiltersAndCaps(t *testing.T) {
	rt := dnsQueriesTestRouter(true)
	rec := httptest.NewRecorder()
	rt.handleListDNSQueries(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dns/queries?rcode=nxdomain&app=web&since=1h&limit=999999", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out dnsQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || len(out.Queries) != 1 || out.Queries[0].QName != "nope.acme." || out.Queries[0].Source != "" {
		t.Fatalf("unexpected body %+v", out)
	}

	bad := httptest.NewRecorder()
	rt.handleListDNSQueries(bad, httptest.NewRequest(http.MethodGet, "/api/v1/dns/queries?since=yesterday", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad since = %d", bad.Code)
	}
}

func TestDNSStatsHandlerAndDisabled(t *testing.T) {
	rec := httptest.NewRecorder()
	dnsQueriesTestRouter(true).handleDNSStats(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dns/stats", nil))
	var out dnsStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Total.Queries != 2 || out.Total.NXDomain != 1 || len(out.Apps) != 1 {
		t.Fatalf("stats %+v", out)
	}

	rec = httptest.NewRecorder()
	(&Router{}).handleDNSStats(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dns/stats", nil))
	if rec.Code != http.StatusOK || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("nil log: %d %s", rec.Code, rec.Body)
	}
}
