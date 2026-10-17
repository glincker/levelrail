package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/clitree"
)

func cacheServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = append(*seen, r.Method+" "+r.URL.Path+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/web/cache":
			_ = json.NewEncoder(w).Encode(apiclient.AppCacheResource{App: "web", Domains: []apiclient.AppCacheDomain{{Domain: "a.test", Enabled: true, Rules: 1}}})
		case strings.HasSuffix(r.URL.Path, "/cache/purge"):
			_, _ = w.Write([]byte(`{"app":"web","scope":"tag","value":"blog","removed":3,"domains":[]}`))
		case strings.HasSuffix(r.URL.Path, "/cache/purges"):
			_, _ = w.Write([]byte(`{"app":"web","purges":[{"domain":"a.test","scope":"all","removed":2,"actor":"root","at":"2026-10-17T10:00:00Z"}]}`))
		case strings.HasSuffix(r.URL.Path, "/cache/preset"):
			_, _ = w.Write([]byte(`{"app":"web","domains":[{"domain":"a.test","applied":true,"rules":1}]}`))
		case r.URL.Path == "/api/v1/system/cache":
			_, _ = w.Write([]byte(`{"limits":{"max_bytes":100,"used_bytes":10},"totals":{"hit_rate":0.5},"apps":[{"app":"web","domains":1,"enabled_domains":1,"requests":10,"hit_rate":0.5}]}`))
		default:
			_, _ = w.Write([]byte(`{"domain":"a.test","configured":true,"spec":{"enabled":true,"rules":[{"match":{"kind":"prefix","path":"/s"},"ttl_seconds":60}]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_Cache(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		call string
	}{
		{"status instance", []string{"cache", "status"}, "web", "GET /api/v1/system/cache"},
		{"status app", []string{"cache", "status", "web"}, "a.test: cache on", "GET /api/v1/apps/web/cache"},
		{"purge tag", []string{"cache", "purge", "web", "--tag", "blog"}, "3 cached responses purged", `"scope":"tag","value":"blog"`},
		{"purge all one domain", []string{"cache", "purge", "web", "--all", "--domain", "a.test"}, "purged", `"domain":"a.test"`},
		{"history", []string{"cache", "history", "web"}, "root", "GET /api/v1/apps/web/cache/purges"},
		{"preset", []string{"cache", "rules", "set", "web", "--preset", "static"}, "preset applied", "POST /api/v1/apps/web/cache/preset"},
		{"rules show", []string{"cache", "rules", "show", "web"}, "prefix /s: ttl 60s", "GET /api/v1/apps/web/domains/a.test/cache"},
		{"rules clear", []string{"cache", "rules", "clear", "web"}, "turned off", "DELETE /api/v1/apps/web/domains/a.test/cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			srv := cacheServer(t, &seen)
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tt.want) {
				t.Errorf("stdout %q missing %q", stdout, tt.want)
			}
			if !strings.Contains(strings.Join(seen, "\n"), tt.call) {
				t.Errorf("calls %v missing %q", seen, tt.call)
			}
		})
	}
}

func TestRun_CachePurgeNeedsExactlyOneScope(t *testing.T) {
	expectCacheUsageError(t, []string{"cache", "purge", "web"})
	expectCacheUsageError(t, []string{"cache", "purge", "web", "--all", "--tag", "x"})
	expectCacheUsageError(t, []string{"cache", "rules", "set", "web"})
}

func expectCacheUsageError(t *testing.T, args []string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("%v exit = %d, want %d (stderr=%q)", args, got, exitUsage, stderr.String())
	}
}

func TestCacheIsInTheCompletionTree(t *testing.T) {
	node := clitree.Tree["cache"]
	if node == nil {
		t.Fatal("cache missing from the completion tree")
	}
	for _, v := range []string{"status", "purge", "history", "rules"} {
		if _, ok := node.Subs[v]; !ok {
			t.Errorf("cache %s missing", v)
		}
	}
	for _, v := range []string{"show", "set", "clear"} {
		if _, ok := node.Subs["rules"].Subs[v]; !ok {
			t.Errorf("cache rules %s missing", v)
		}
	}
}
