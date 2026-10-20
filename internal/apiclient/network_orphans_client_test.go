package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_NetworkSharesTopologyAndOrphans(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/network-shares":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[]`))
				return
			}
		case "/api/v1/system/containers/c1/stop", "/api/v1/system/containers/c1/remove",
			"/api/v1/network-shares/s1", "/api/v1/network-shares/s1/test":
			if r.Method != http.MethodGet && r.Method != http.MethodPut {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	ctx := context.Background()

	cases := []struct {
		name, method, path string
		call               func() error
	}{
		{"create", http.MethodPost, "/api/v1/network-shares", func() error { _, e := c.CreateNetworkShare(ctx, CreateNetworkShareRequest{}); return e }},
		{"list", http.MethodGet, "/api/v1/network-shares", func() error { _, e := c.ListNetworkShares(ctx); return e }},
		{"get", http.MethodGet, "/api/v1/network-shares/s1", func() error { _, e := c.GetNetworkShare(ctx, "s1"); return e }},
		{"update", http.MethodPut, "/api/v1/network-shares/s1", func() error { _, e := c.UpdateNetworkShare(ctx, "s1", UpdateNetworkShareRequest{}); return e }},
		{"delete", http.MethodDelete, "/api/v1/network-shares/s1", func() error { return c.DeleteNetworkShare(ctx, "s1") }},
		{"test", http.MethodPost, "/api/v1/network-shares/s1/test", func() error { return c.TestNetworkShare(ctx, "s1") }},
		{"topology", http.MethodGet, "/api/v1/network/topology", func() error { _, e := c.GetNetworkTopology(ctx); return e }},
		{"proxy", http.MethodGet, "/api/v1/network/proxy", func() error { _, e := c.GetNetworkProxyStatus(ctx); return e }},
		{"project topology", http.MethodGet, "/api/v1/projects/p1/topology", func() error { _, e := c.GetProjectTopology(ctx, "p1"); return e }},
		{"stop", http.MethodPost, "/api/v1/system/containers/c1/stop", func() error { return c.StopOrphanedContainer(ctx, "c1") }},
		{"remove", http.MethodPost, "/api/v1/system/containers/c1/remove", func() error { return c.RemoveOrphanedContainer(ctx, "c1") }},
		{"claim", http.MethodPost, "/api/v1/system/containers/c1/claim", func() error {
			_, e := c.ClaimOrphanedContainer(ctx, "c1", ClaimOrphanedContainerRequest{Name: "x"})
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("error: %v", err)
			}
			if gotMethod != tc.method || gotPath != tc.path {
				t.Errorf("got %s %s, want %s %s", gotMethod, gotPath, tc.method, tc.path)
			}
		})
	}
}
