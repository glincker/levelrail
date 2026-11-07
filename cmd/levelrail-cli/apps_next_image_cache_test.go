package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsNextImageCacheSet(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&sent)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "x", "port": 3000})
	}))
	t.Cleanup(srv.Close)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "next-image-cache", "set", "web", "persistent", "--api-url", srv.URL})
	if sent["next_image_cache"] != "persistent" {
		t.Fatalf("PUT body next_image_cache = %v", sent["next_image_cache"])
	}
	if !strings.Contains(stdout, "next.image_cache set to persistent") {
		t.Errorf("stdout = %q", stdout)
	}
}
