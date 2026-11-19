package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPagesArgs_SplitsDomainAndDir(t *testing.T) {
	domain, rest, dir := pagesArgs([]string{"--name", "blog", "--domain=blog.example.com", "./site", "--yes"})
	if domain != "blog.example.com" || dir != "./site" {
		t.Fatalf("domain=%q dir=%q", domain, dir)
	}
	if want := []string{"--name", "blog", "./site", "--yes"}; !reflect.DeepEqual(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}

func TestLooksLikeStaticSite(t *testing.T) {
	dir := t.TempDir()
	if looksLikeStaticSite(dir) {
		t.Fatal("empty dir is not a site")
	}
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600)
	if !looksLikeStaticSite(dir) {
		t.Fatal("index.html alone should look static")
	}
	_ = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x"), 0o600)
	if looksLikeStaticSite(dir) {
		t.Fatal("a Dockerfile means a container deploy")
	}
}

func TestPreviousPagesDeploy(t *testing.T) {
	now := time.Now()
	got := previousPagesDeploy([]deployAttemptResource{
		{ID: "old", Status: "succeeded", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "live", Status: "succeeded", Live: true, StartedAt: now},
		{ID: "prev", Status: "succeeded", StartedAt: now.Add(-time.Hour)},
		{ID: "bad", Status: "failed", StartedAt: now.Add(-time.Minute)},
	})
	if got != "prev" {
		t.Errorf("got %q, want prev", got)
	}
}

func TestRun_Pages_DomainsAddAndShow(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/domains") {
			_ = json.NewEncoder(w).Encode(staticSiteDomain{Domain: "a.example.com", Mode: "manual", Records: nil})
			return
		}
		_ = json.NewEncoder(w).Encode(staticSiteDetail{Name: "blog", Published: true, FileCount: 3})
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"pages", "domains", "add", "blog", "a.example.com", "--api-url", srv.URL})
	if !strings.Contains(out, "a.example.com") {
		t.Errorf("stdout = %q", out)
	}
	out, _ = runCLIExpectOK(t, []string{"pages", "show", "blog", "--api-url", srv.URL})
	if !strings.Contains(out, "published:  true") {
		t.Errorf("stdout = %q", out)
	}
	want := []string{"POST /api/v1/static-sites/blog/domains", "GET /api/v1/static-sites/blog"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestRun_Pages_SettingsSetAndRoutesAdd(t *testing.T) {
	var calls []string
	var patch map[string]any
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodPatch:
			_ = json.NewDecoder(r.Body).Decode(&patch)
			_, _ = w.Write([]byte(`{"name":"docs","spa_fallback":true,"effective":{"spa":true}}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&put)
			_, _ = w.Write([]byte(`{"ui":{"redirects":[{"from":"/old","to":"/new","status":302}],"headers":[]}}`))
		default:
			_, _ = w.Write([]byte(`{"ui":{"redirects":[],"headers":[]}}`))
		}
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"pages", "settings", "set", "docs", "--spa", "on", "--404", "auto", "--api-url", srv.URL})
	if patch["spa_fallback"] != true || patch["reset"].([]any)[0] != "not_found_page" {
		t.Fatalf("patch = %v", patch)
	}
	runCLIExpectOK(t, []string{"pages", "routes", "add", "docs", "/old", "/new", "--status", "302", "--api-url", srv.URL})
	if r := put["redirects"].([]any); len(r) != 1 || r[0].(map[string]any)["to"] != "/new" {
		t.Fatalf("put = %v (calls %v)", put, calls)
	}
}
