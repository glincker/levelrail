package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReverseProxyGuide(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.dashboardListenAddr = "127.0.0.1:18080"
	cookie := loginTestSession(t, rt, db)

	get := func(query string) (int, reverseProxyGuideResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/reverse-proxy"+query, ""))
		var out reverseProxyGuideResource
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, out := get("")
	if code != http.StatusOK || out.Holders == nil || out.Plan != nil {
		t.Fatalf("no domain: code=%d holders=%v plan=%v, want 200, an empty list and no plan", code, out.Holders, out.Plan)
	}

	code, out = get("?domain=console.example.com&proxy=nginx")
	if code != http.StatusOK || out.Plan == nil {
		t.Fatalf("with domain: code=%d plan=%v", code, out.Plan)
	}
	if out.Plan.UpstreamURL != "http://127.0.0.1:18080" || out.Plan.NeedsRebind {
		t.Errorf("host proxy plan = %+v, want a loopback upstream and no rebind", out.Plan)
	}

	if code, _ = get("?domain=not-a-host"); code != http.StatusBadRequest {
		t.Errorf("bad domain code = %d, want 400", code)
	}
	if code, _ = get("?domain=console.example.com&proxy=apache"); code != http.StatusBadRequest {
		t.Errorf("unknown proxy code = %d, want 400", code)
	}
}
