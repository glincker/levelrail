package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLBStats map[string]loadbalancer.Stats

func (f fakeLBStats) UpstreamStats(context.Context) (map[string]loadbalancer.Stats, error) {
	return f, nil
}

type okProber struct{}

func (okProber) Probe(context.Context, string, string, *loadbalancer.UpstreamTLS, time.Duration) loadbalancer.ProbeResult {
	return loadbalancer.ProbeResult{OK: true, StatusCode: 200, CheckedAt: time.Now()}
}

func newLBRouter(t *testing.T, reg *loadbalancer.Registry, stats loadbalancer.StatsSource) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithLoadBalancers(db, reg, stats))
	rt.lb.prober = okProber{}
	return rt, db, loginTestSession(t, rt, db)
}

func lbDo(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestLoadBalancerRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/loadbalancer"},
		{http.MethodPut, "/api/v1/apps/web/loadbalancer"},
		{http.MethodDelete, "/api/v1/apps/web/loadbalancer"},
		{http.MethodPost, "/api/v1/apps/web/loadbalancer/import"},
		{http.MethodGet, "/api/v1/apps/web/loadbalancer/status"},
		{http.MethodGet, "/api/v1/apps/web/loadbalancer/export?format=cdk"},
	})
}

func TestLoadBalancerRoutes_NotConfiguredReturns501(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestLoadBalancer_SetGetDeleteLifecycle(t *testing.T) {
	rt, db, cookie := newLBRouter(t, nil, nil)
	seedWebAppForTest(t, db)

	rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer", "")
	var res loadBalancerResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Configured || len(res.Algorithms) != 6 {
		t.Fatalf("unconfigured GET = %d %s", rec.Code, rec.Body.String())
	}

	body := `{"algorithm":"weighted","weights":[3,1],"active_health":{"path":"/healthz","interval":"5s","timeout":"2s"}}`
	if rec := lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer", body); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	rec = lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer", "")
	res = loadBalancerResource{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Configured || res.Config.Algorithm != "weighted" || len(res.Config.Weights) != 2 {
		t.Fatalf("GET after PUT = %s", rec.Body.String())
	}

	if rec := lbDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/loadbalancer", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	rec = lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer", "")
	if !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("GET after DELETE = %s", rec.Body.String())
	}
}

func TestLoadBalancer_SetRejectsBadInput(t *testing.T) {
	rt, db, cookie := newLBRouter(t, nil, nil)
	seedWebAppForTest(t, db)
	tests := []struct {
		name, target, body string
		want               int
	}{
		{"bad algorithm", "/api/v1/apps/web/loadbalancer", `{"algorithm":"random"}`, http.StatusBadRequest},
		{"unknown field", "/api/v1/apps/web/loadbalancer", `{"sticky":true}`, http.StatusBadRequest},
		{"not json", "/api/v1/apps/web/loadbalancer", `nope`, http.StatusBadRequest},
		{"missing app", "/api/v1/apps/ghost/loadbalancer", `{}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := lbDo(t, rt, cookie, http.MethodPut, tt.target, tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestLoadBalancer_Import(t *testing.T) {
	rt, db, cookie := newLBRouter(t, nil, nil)
	seedWebAppForTest(t, db)

	const doc = "version: 1\nservices:\n  web:\n    build: { type: dockerfile }\n    port: 80\n    loadbalancer: { algorithm: ip_hash }\n  api:\n    build: { type: dockerfile }\n    port: 81\n"
	if rec := lbDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/loadbalancer/import", doc); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ip_hash") {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	if rec := lbDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/loadbalancer/import?service=api", doc); rec.Code != http.StatusBadRequest {
		t.Fatalf("import service without block = %d, want 400", rec.Code)
	}
	if rec := lbDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/loadbalancer/import", "not: [valid"); rec.Code != http.StatusBadRequest {
		t.Fatalf("import bad yaml = %d, want 400", rec.Code)
	}
}

func TestLoadBalancer_StatusMergesObservationAndStats(t *testing.T) {
	reg := loadbalancer.NewRegistry()
	rt, db, cookie := newLBRouter(t, reg, fakeLBStats{"127.0.0.1:1": {NumRequests: 7}})
	seedWebAppForTest(t, db)

	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status unconfigured = %d, want 404", rec.Code)
	}
	if rec := lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer", `{"active_health":{"path":"/h"}}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d", rec.Code)
	}
	rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", "")
	if !strings.Contains(rec.Body.String(), `"reason":"Pending"`) {
		t.Fatalf("before first pass = %s", rec.Body.String())
	}

	reg.Record(loadbalancer.Observation{
		Service: "web", Ready: true, Reason: "Balancing", ObservedAt: time.Now(),
		Upstreams: []loadbalancer.UpstreamObservation{{Upstream: loadbalancer.Upstream{ID: "web#0", Dial: "127.0.0.1:1"}, Running: true}},
	})
	rec = lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", "")
	var st loadbalancer.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || len(st.Upstreams) != 1 || st.Upstreams[0].State != loadbalancer.StateHealthy || st.Upstreams[0].ActiveConns != 7 {
		t.Fatalf("status = %s", rec.Body.String())
	}
}

func TestLoadBalancer_Export(t *testing.T) {
	reg := loadbalancer.NewRegistry()
	rt, db, cookie := newLBRouter(t, reg, nil)
	seedWebAppForTest(t, db)

	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/export?format=cdk", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("export unconfigured = %d, want 404", rec.Code)
	}
	if rec := lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer", `{"algorithm":"least_conn"}`); rec.Code != http.StatusOK {
		t.Fatal("PUT failed")
	}
	rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/export?format=terraform", "")
	var art struct{ Filename, Body string }
	if err := json.Unmarshal(rec.Body.Bytes(), &art); err != nil || art.Filename != "web-lb.tf" || !strings.Contains(art.Body, "least_outstanding_requests") {
		t.Fatalf("export = %d %s", rec.Code, rec.Body.String())
	}
	raw := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/export?format=caddy&raw=true", "")
	if !strings.Contains(raw.Header().Get("Content-Disposition"), "Caddyfile") || !strings.Contains(raw.Body.String(), "lb_policy least_conn") {
		t.Fatalf("raw export = %v %s", raw.Header(), raw.Body.String())
	}
	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/export?format=pulumi", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown format = %d, want 400", rec.Code)
	}
}
