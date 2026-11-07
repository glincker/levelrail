package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsCache(t *testing.T) {
	now := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/build-cache" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			_ = json.NewEncoder(w).Encode(apiclient.AppBuildCacheClearResult{CacheID: "web", FreedBytes: 2048, RemovedCount: 3})
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.AppBuildCache{CacheID: "web", SizeBytes: 2048, RecordCount: 3, LastUsedAt: &now})
	}))
	t.Cleanup(srv.Close)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"show":  {[]string{"apps", "cache", "show", "web"}, "3 records"},
		"clear": {[]string{"apps", "cache", "clear", "web"}, "3 records removed"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("stdout %q missing %q", stdout, tc.want)
			}
		})
	}
}
