package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ProxyIntegration(t *testing.T) {
	var gotMethod, gotURI, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotURI, gotBody = r.Method, r.URL.RequestURI(), string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"detected": map[string]any{"kind": "traefik", "complete": true}, "domains": []any{map[string]any{"domain": "a.example.com", "proxy_loaded": true}}})
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "t")
	ctx := context.Background()
	tests := []struct {
		name                 string
		call                 func() error
		method, uri, wantBdy string
	}{
		{"get", func() error { _, err := c.GetProxyIntegration(ctx); return err }, http.MethodGet, "/api/v1/system/proxy-integration", ""},
		{"setup dry run", func() error { _, err := c.SetupProxyIntegration(ctx, ProxySetupRequest{}); return err }, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":false}`},
		{"setup confirm with dir", func() error {
			_, err := c.SetupProxyIntegration(ctx, ProxySetupRequest{Confirm: true, DynamicDir: "/d"})
			return err
		}, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true,"dynamic_dir":"/d"}`},
		{"apply", func() error { _, err := c.ApplyProxyIntegration(ctx); return err }, http.MethodPost, "/api/v1/system/proxy-integration/apply", ""},
		{"verify one", func() error { _, err := c.VerifyProxyIntegration(ctx, "a.example.com"); return err }, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=a.example.com", ""},
		{"disable", func() error {
			_, err := c.UpdateProxyIntegrationSettings(ctx, ProxyIntegrationSettings{Integration: "off"})
			return err
		}, http.MethodPut, "/api/v1/settings/proxy-integration", `{"integration":"off","dynamic_dir":"","entrypoint_http":"","entrypoint_https":"","cert_resolver":"","upstream_host":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatal(err)
			}
			if gotMethod != tc.method || gotURI != tc.uri || gotBody != tc.wantBdy {
				t.Errorf("got %s %s %q", gotMethod, gotURI, gotBody)
			}
		})
	}
	got, err := c.GetProxyIntegration(ctx)
	if err != nil || got.Detected.Kind != "traefik" || len(got.Domains) != 1 || got.Domains[0].ProxyLoaded == nil || !*got.Domains[0].ProxyLoaded {
		t.Errorf("decoded %+v, %v", got, err)
	}
}
