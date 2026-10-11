package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestDomainPolicyClientRoutes(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{"get headers", func(c *Client) error { _, err := c.GetDomainHeaders(ctx, "web", "a.example.com"); return err }, http.MethodGet, "/api/v1/apps/web/domains/a.example.com/headers", "", ""},
		{"set headers", func(c *Client) error {
			_, err := c.SetDomainHeaders(ctx, "web", "a.example.com", trafficpolicy.Headers{Rules: []trafficpolicy.HeaderRule{{Side: "response", Op: "set", Name: "X-A", Value: "1"}}})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/headers", "", `"name":"X-A"`},
		{"clear headers", func(c *Client) error { _, err := c.ClearDomainHeaders(ctx, "web", "a.example.com"); return err }, http.MethodDelete, "/api/v1/apps/web/domains/a.example.com/headers", "", ""},
		{"set forwarders", func(c *Client) error {
			_, err := c.SetDomainForwarders(ctx, "web", "a.example.com", trafficpolicy.Forwarders{Rules: []trafficpolicy.Forwarder{{Match: trafficpolicy.Match{Kind: "prefix", Path: "/api"}, Action: "app", App: "api"}}})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/forwarders", "", `"app":"api"`},
		{"set geo", func(c *Client) error {
			_, err := c.SetDomainGeo(ctx, "web", "a.example.com", trafficpolicy.Geo{Mode: "deny", Countries: []string{"RU"}, Action: "block"})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/geo", "", `"countries":["RU"]`},
		{"clear cache", func(c *Client) error { _, err := c.ClearDomainCache(ctx, "web", "a.example.com"); return err }, http.MethodDelete, "/api/v1/apps/web/domains/a.example.com/cache", "", ""},
		{"policies", func(c *Client) error { _, err := c.GetDomainPolicies(ctx, "web", "a.example.com"); return err }, http.MethodGet, "/api/v1/apps/web/domains/a.example.com/policies", "", ""},
		{"preview", func(c *Client) error {
			_, err := c.PreviewDomainPolicies(ctx, "web", "a.example.com", PolicyPreviewRequest{Method: "GET", Path: "/x"})
			return err
		}, http.MethodPost, "/api/v1/apps/web/domains/a.example.com/policies/preview", "", `"path":"/x"`},
		{"purge", func(c *Client) error {
			_, err := c.PurgeDomainCache(ctx, "web", "a.example.com", PurgeCacheRequest{Scope: "prefix", Value: "/static"})
			return err
		}, http.MethodPost, "/api/v1/apps/web/domains/a.example.com/cache/purge", "", `"scope":"prefix"`},
		{"cache stats", func(c *Client) error { _, err := c.GetDomainCacheStats(ctx, "web", "a.example.com"); return err }, http.MethodGet, "/api/v1/apps/web/domains/a.example.com/cache/stats", "", ""},
		{"redirects", func(c *Client) error { _, err := c.GetDomainRedirects(ctx, "web", "a.example.com"); return err }, http.MethodGet, "/api/v1/apps/web/domains/a.example.com/redirects", "", ""},
		{"redirect settings", func(c *Client) error {
			_, err := c.SetDomainRedirectSettings(ctx, "web", "a.example.com", trafficpolicy.Redirects{ForceHTTPS: true})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/redirects", "", `"force_https":true`},
		{"canonical", func(c *Client) error {
			_, err := c.SetDomainCanonical(ctx, "web", "a.example.com", CanonicalRequest{Preset: "www-to-apex"})
			return err
		}, http.MethodPost, "/api/v1/apps/web/domains/a.example.com/redirects/canonical", "", `"preset":"www-to-apex"`},
		{"aliases", func(c *Client) error {
			_, err := c.SetDomainAliases(ctx, "web", "a.example.com", AliasesRequest{Aliases: []string{"b.example.com"}, StatusCode: 308})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/redirects/aliases", "", `"status_code":308`},
		{"ports", func(c *Client) error { _, err := c.GetDomainPorts(ctx, "web", "a.example.com"); return err }, http.MethodGet, "/api/v1/apps/web/domains/a.example.com/ports", "", ""},
		{"restrict", func(c *Client) error {
			_, err := c.RestrictDomainPort(ctx, "web", "a.example.com", 5432, []string{"10.0.0.0/8"})
			return err
		}, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/ports/5432/restrict", "", `"sources":["10.0.0.0/8"]`},
		{"unrestrict", func(c *Client) error { _, err := c.UnrestrictDomainPort(ctx, "web", "a.example.com", 5432); return err }, http.MethodDelete, "/api/v1/apps/web/domains/a.example.com/ports/5432/restrict", "", ""},
		{"geoip", func(c *Client) error { _, err := c.GeoIPLookup(ctx, "8.8.8.8"); return err }, http.MethodGet, "/api/v1/system/geoip", "ip=8.8.8.8", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, path, query, body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			if err := tt.call(NewClient(srv.URL, "tok")); err != nil {
				t.Fatalf("call error = %v", err)
			}
			if method != tt.wantMethod || path != tt.wantPath || query != tt.wantQuery {
				t.Errorf("request = %s %s?%s, want %s %s?%s", method, path, query, tt.wantMethod, tt.wantPath, tt.wantQuery)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", body, tt.wantBody)
			}
		})
	}
}

func TestDomainPolicyClientDecodesSpec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"domain":"a.example.com","kind":"cache","configured":true,"spec":{"enabled":true,"rules":[{"match":{"kind":"prefix","path":"/static"},"ttl_seconds":60}]}}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, "tok").GetDomainCache(context.Background(), "web", "a.example.com")
	if err != nil {
		t.Fatalf("GetDomainCache() error = %v", err)
	}
	if !got.Configured || !got.Spec.Enabled || len(got.Spec.Rules) != 1 || got.Spec.Rules[0].TTLSeconds != 60 {
		t.Errorf("decoded = %+v", got)
	}
}
