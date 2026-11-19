package main

import (
	"bytes"
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

func TestRun_Pages_DeployGitFlags(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "site", "deploy_id": "dep_1"})
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"pages", "deploy", "--git", "https://example.com/org/site.git", "--branch", "dev", "--build-command", "make", "--output-dir", "out", "--api-url", srv.URL})
	git, _ := got["git"].(map[string]any)
	if got["name"] != "site" || got["deploy"] != true || git["repo_url"] != "https://example.com/org/site.git" || git["branch"] != "dev" || git["build_command"] != "make" || git["output_dir"] != "out" {
		t.Errorf("request = %v", got)
	}
	if !strings.Contains(out, "deploy dep_1") {
		t.Errorf("stdout = %q", out)
	}
	runCLIExpectOK(t, []string{"pages", "deploy", "--git", "https://example.com/org/site.git", "--name", "custom", "--api-url", srv.URL})
	if got["name"] != "custom" {
		t.Errorf("--name ignored: %v", got["name"])
	}
}

func TestRun_Pages_SettingsPasswordGate(t *testing.T) {
	var patch map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		patch = nil
		_ = json.NewDecoder(r.Body).Decode(&patch)
		_, _ = w.Write([]byte(`{"name":"docs","protected":true,"effective":{}}`))
	}))
	defer srv.Close()
	var outBuf, errBuf bytes.Buffer
	env := map[string]string{"PAGES_GATE_PW": "correct horse battery"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if got := run("levelrail-cli-test", []string{"pages", "settings", "set", "docs", "--password-env", "PAGES_GATE_PW", "--api-url", srv.URL}, &outBuf, &errBuf, lookup); got != exitOK {
		t.Fatalf("exit = %d stderr=%q", got, errBuf.String())
	}
	if patch["password"] != "correct horse battery" {
		t.Fatalf("patch = %v", patch)
	}
	runCLIExpectOK(t, []string{"pages", "settings", "set", "docs", "--clear-password", "--api-url", srv.URL})
	if v, ok := patch["password"]; !ok || v != "" {
		t.Fatalf("clear patch = %v", patch)
	}
}

func TestRun_Pages_SettingsPasswordFlag(t *testing.T) {
	var patch map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		patch = nil
		_ = json.NewDecoder(r.Body).Decode(&patch)
		_, _ = w.Write([]byte(`{"name":"docs","protected":true,"effective":{}}`))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"pages", "settings", "set", "docs", "--password", "hunter two", "--api-url", srv.URL})
	if patch["password"] != "hunter two" {
		t.Fatalf("patch = %v", patch)
	}
	runCLIExpectValidationError(t, []string{"pages", "settings", "set", "docs", "--password", "x", "--clear-password", "--api-url", srv.URL})
}

func TestRun_Pages_HookCreateRotateIntent(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["rotate"] == true {
				bodies = append(bodies, "rotate")
			} else {
				bodies = append(bodies, "create")
			}
			_, _ = w.Write([]byte(`{"enabled":true,"url":"https://lr.example.com/api/v1/hooks/pages/docs/abc"}`))
			return
		}
		_, _ = w.Write([]byte(`{"enabled":false}`))
	}))
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"pages", "hook", "create", "docs", "--api-url", srv.URL})
	if !strings.Contains(out, "/hooks/pages/docs/abc") {
		t.Errorf("create stdout = %q", out)
	}
	runCLIExpectOK(t, []string{"pages", "hook", "rotate", "docs", "--api-url", srv.URL})
	if len(bodies) != 2 || bodies[0] != "create" || bodies[1] != "rotate" {
		t.Fatalf("intents = %v, want [create rotate]", bodies)
	}
	out, _ = runCLIExpectOK(t, []string{"pages", "hook", "show", "docs", "--api-url", srv.URL})
	if !strings.Contains(out, "no deploy hook") {
		t.Errorf("show stdout = %q", out)
	}
}

func TestPagesLogs_LatestDeployAndSteps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/static-sites/blog/deploys":
			_, _ = w.Write([]byte(`{"deploys":[{"id":"old","started_at":"2026-01-01T00:00:00Z"},{"id":"new","started_at":"2026-02-01T00:00:00Z"}]}`))
		case "/api/v1/static-sites/blog/deploys/new/log":
			_, _ = w.Write([]byte(`{"deploy_id":"new","lines":[{"step":"building","stream":"stdout","line":"npm ci"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"pages", "logs", "blog", "--api-url", srv.URL})
	if !strings.Contains(out, "== building") || !strings.Contains(out, "stdout npm ci") {
		t.Fatalf("out = %q", out)
	}
}
