package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsNextCacheSet(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&sent)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "x", "port": 3000})
	}))
	t.Cleanup(srv.Close)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "next-cache", "set", "web", "redis", "--api-url", srv.URL})
	if sent["next_cache"] != "redis" {
		t.Fatalf("PUT body next_cache = %v", sent["next_cache"])
	}
	if !strings.Contains(stdout, "next.cache set to redis") {
		t.Errorf("stdout = %q", stdout)
	}
}
