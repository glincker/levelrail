package api

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/pages"
	"github.com/GLINCKER/levelrail/internal/trafficclass"
)

func TestStaticSiteTraffic(t *testing.T) {
	log := ingress.NewAccessLog(100)
	now := time.Now()
	rec := func(path string, status int, out int64, ip string, ago time.Duration) {
		log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-ago), Host: "web.example.com", Path: path, Status: status, BytesOut: out, Client: netip.MustParseAddr(ip)})
	}
	rec("/", 200, 1000, "192.0.2.1", time.Minute)
	rec("/missing", 404, 50, "192.0.2.2", 2*time.Minute)
	rec("/missing", 404, 50, "192.0.2.3", 3*time.Minute)
	rec("/about/", 200, 10, "192.0.2.1", time.Minute)
	rec("/About", 200, 10, "192.0.2.4", time.Minute)
	rec("/assets/app-abc123.js", 200, 4000, "192.0.2.1", time.Minute)
	rec("/.env", 404, 5, "192.0.2.9", time.Minute)
	rec("/wp-login.php", 404, 5, "192.0.2.9", time.Minute)
	rt, cookie := visitorsRouter(t, log)

	get := func(q string) (int, staticSiteTrafficResponse) {
		r := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/traffic"+q, "")
		var res staticSiteTrafficResponse
		_ = json.Unmarshal(r.Body.Bytes(), &res)
		return r.Code, res
	}
	code, res := get("")
	if code != http.StatusOK || res.Window != "24h" || res.Requests != 8 || res.BytesOut != 5130 || res.Status["4xx"] != 4 {
		t.Fatalf("24h = %d %+v", code, res)
	}
	if len(res.Top404Paths) != 3 || res.Top404Paths[0].Label != "/missing" || res.Top404Paths[0].Count != 2 || res.Partial {
		t.Fatalf("404s = %+v", res)
	}
	if len(res.BrokenLinks) != 1 || res.BrokenLinks[0].Label != "/missing" || len(res.Probes) != 2 {
		t.Fatalf("split = %+v %+v", res.BrokenLinks, res.Probes)
	}
	if res.Assets.Requests != 1 || res.Assets.Bytes != 4000 {
		t.Fatalf("assets = %+v", res.Assets)
	}
	for _, p := range res.TopPages {
		if p.Label == "/assets/app-abc123.js" || p.Label == "/.env" {
			t.Fatalf("non-page in top_pages: %+v", res.TopPages)
		}
	}
	if len(res.TopPages) == 0 || len(res.TopPaths) == 0 {
		t.Fatalf("pages = %+v", res)
	}
	if code, res = get("?window=7d"); code != http.StatusOK || res.Window != "7d" {
		t.Fatalf("7d = %d %+v", code, res)
	}
	if code, _ = get("?window=1h"); code != http.StatusBadRequest {
		t.Fatalf("bad window = %d", code)
	}
}

func TestStaticSiteTrafficSeries(t *testing.T) {
	t.Setenv("APP_PAGES_TRAFFIC_BUCKETS", "6")
	log := ingress.NewAccessLog(100)
	now := time.Now()
	add := func(path string, status int, class trafficclass.Class, ago time.Duration) {
		log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-ago), Host: "web.example.com", Path: path, Status: status, Class: class, Client: netip.MustParseAddr("192.0.2.1")})
	}
	add("/", 200, trafficclass.Page, time.Minute)
	add("/a.js", 200, trafficclass.Asset, time.Minute)
	add("/gone", 404, trafficclass.Page, 20*time.Hour)
	rt, cookie := visitorsRouter(t, log)
	r := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/traffic?range=24h", "")
	var res staticSiteTrafficResponse
	if err := json.Unmarshal(r.Body.Bytes(), &res); err != nil || len(res.Series) != 6 {
		t.Fatalf("series = %d %s", r.Code, r.Body.String())
	}
	var req, pages, assets, errs int
	for _, p := range res.Series {
		req, pages, assets, errs = req+p.Requests, pages+p.Pages, assets+p.Assets, errs+p.Errors
	}
	if req != 3 || pages != 2 || assets != 1 || errs != 1 || res.Series[5].Requests != 2 {
		t.Fatalf("totals = %d %d %d %d %+v", req, pages, assets, errs, res.Series)
	}
	if r = doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/traffic?range=30d", ""); r.Code != http.StatusOK {
		t.Fatalf("30d = %d", r.Code)
	}
}

func TestStaticSiteAnalyticsPrivacy(t *testing.T) {
	log := ingress.NewAccessLog(100)
	now := time.Now()
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-time.Minute), Host: "web.example.com", Path: "/about?token=secret", Status: 200, Referrer: "news.example.org", Client: netip.MustParseAddr("192.0.2.1")})
	log.Record("web.example.com", ingress.AccessRecord{Time: now.Add(-time.Minute), Host: "web.example.com", Path: "/gone", Status: 404, Client: netip.MustParseAddr("192.0.2.2")})
	rt, cookie := visitorsRouter(t, log)
	r := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/analytics?range=7d", "")
	if r.Code != http.StatusOK {
		t.Fatalf("code = %d %s", r.Code, r.Body.String())
	}
	body := r.Body.String()
	for _, bad := range []string{"token", "secret", "192.0.2"} {
		if strings.Contains(body, bad) {
			t.Fatalf("leaked %q: %s", bad, body)
		}
	}
	var res pages.Analytics
	_ = json.Unmarshal(r.Body.Bytes(), &res)
	if len(res.Status) != 2 || res.RetentionDays != 30 {
		t.Fatalf("res = %+v", res)
	}
	if r = doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/analytics?range=1h", ""); r.Code != http.StatusBadRequest {
		t.Fatalf("bad range = %d", r.Code)
	}
}
