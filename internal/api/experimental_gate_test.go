package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// TestMain enables every experimental feature so the rest of the package
// exercises the gated handlers; the gate tests below narrow it per case.
func TestMain(m *testing.M) {
	experimental.Set(experimental.All()...)
	os.Exit(m.Run())
}

func TestExperimentalGateMiddleware(t *testing.T) {
	tests := []struct {
		method, path string
		feature      experimental.Feature
	}{
		{http.MethodPost, "/api/v1/ai/sessions", experimental.AIChat},
		{http.MethodGet, "/api/v1/settings/ai-assistant", experimental.AIChat},
		{http.MethodGet, "/api/v1/models", experimental.AIModels},
		{http.MethodGet, "/api/v1/models/m1/keys", experimental.AIModels},
		{http.MethodGet, "/api/v1/loadbalancers", experimental.LoadBalancer},
		{http.MethodGet, "/api/v1/apps/web/loadbalancer/status", experimental.LoadBalancer},
		{http.MethodPost, "/api/v1/apply/plan", experimental.IaC},
		{http.MethodGet, "/api/v1/export", experimental.IaC},
		{http.MethodGet, "/api/v1/settings/cloudflare-tunnel", experimental.CloudflareTunnel},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			defer experimental.Set(experimental.All()...)
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := experimentalGateMiddleware(next)

			experimental.Set()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("off: status = %d", rec.Code)
			}
			var body experimentalError
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Code != ExperimentalDisabledCode || body.Feature != string(tc.feature) {
				t.Fatalf("body = %+v", body)
			}

			experimental.Set(tc.feature)
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("on: status = %d", rec.Code)
			}
		})
	}
}

func TestExperimentalGateLeavesOtherRoutes(t *testing.T) {
	defer experimental.Set(experimental.All()...)
	experimental.Set()
	for _, p := range []string{"/api/v1/apps", "/api/v1/apps/web/deploys", "/api/v1/settings/cloudflare-dns", "/api/v1/brand"} {
		if f, gated := featureForPath(p); gated {
			t.Errorf("%s gated by %s", p, f)
		}
	}
}
