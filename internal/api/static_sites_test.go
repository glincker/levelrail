package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListStaticSites_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []staticSiteResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d static sites, want 0 for a control plane with none deployed", len(got))
	}
}

func TestHandleListStaticSites_Populated(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	sites := []store.StaticSite{
		{Name: "marketing-site", Domains: []string{"example.com", "www.example.com"}, RootDir: "/var/lib/levelrail-data/static-sites/marketing-site"},
		{Name: "docs-site", Domains: []string{"docs.example.com"}, RootDir: "/var/lib/levelrail-data/static-sites/docs-site"},
	}
	for _, s := range sites {
		if err := db.SaveStaticSite(context.Background(), s); err != nil {
			t.Fatalf("seed static site %q: %v", s.Name, err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []staticSiteResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d static sites, want 2: %+v", len(got), got)
	}

	byName := make(map[string]staticSiteResource, len(got))
	for _, s := range got {
		byName[s.Name] = s
	}

	tests := []struct {
		name    string
		domains []string
	}{
		{"marketing-site", []string{"example.com", "www.example.com"}},
		{"docs-site", []string{"docs.example.com"}},
	}
	for _, tt := range tests {
		s, ok := byName[tt.name]
		if !ok {
			t.Errorf("missing entry for static site %q in %+v", tt.name, got)
			continue
		}
		if len(s.Domains) != len(tt.domains) {
			t.Errorf("site %q: domains = %v, want %v", tt.name, s.Domains, tt.domains)
			continue
		}
		for i, d := range tt.domains {
			if s.Domains[i] != d {
				t.Errorf("site %q: domains[%d] = %q, want %q", tt.name, i, s.Domains[i], d)
			}
		}
	}

	// RootDir is deliberately never sent over the wire (see
	// staticSiteResource's own doc comment): confirm the raw JSON has no
	// such key at all, not just that the Go struct lacks a field for it.
	raw := rec.Body.String()
	if strings.Contains(raw, "root_dir") || strings.Contains(raw, "RootDir") {
		t.Errorf("response body unexpectedly contains a root_dir/RootDir key: %s", raw)
	}
}

func TestStaticSitesRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/static-sites", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}
