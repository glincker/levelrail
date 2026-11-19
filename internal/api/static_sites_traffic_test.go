package api

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
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
	rt, cookie := visitorsRouter(t, log)

	get := func(q string) (int, staticSiteTrafficResponse) {
		r := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/static-sites/web/traffic"+q, "")
		var res staticSiteTrafficResponse
		_ = json.Unmarshal(r.Body.Bytes(), &res)
		return r.Code, res
	}
	code, res := get("")
	if code != http.StatusOK || res.Window != "24h" || res.Requests != 3 || res.BytesOut != 1100 || res.Status["4xx"] != 2 {
		t.Fatalf("24h = %d %+v", code, res)
	}
	if len(res.Top404Paths) != 1 || res.Top404Paths[0].Label != "/missing" || res.Top404Paths[0].Count != 2 || res.Partial {
		t.Fatalf("404s = %+v", res)
	}
	if code, res = get("?window=7d"); code != http.StatusOK || res.Window != "7d" {
		t.Fatalf("7d = %d %+v", code, res)
	}
	if code, _ = get("?window=1h"); code != http.StatusBadRequest {
		t.Fatalf("bad window = %d", code)
	}
}
