package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func newBuildCacheCLIServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/build-cache":
			_ = json.NewEncoder(w).Encode([]apiclient.BuildCacheSetting{{AppName: "web", TargetID: "bkt_1", Enabled: true, Mode: "max", KeyPrefix: "build-cache/web/", LastResult: "fallback", LastWarning: "cache export failed"}})
		case r.Method == http.MethodPut:
			var req apiclient.BuildCacheRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(apiclient.BuildCacheSetting{AppName: req.AppName, TargetID: req.TargetID, Mode: req.Mode, Enabled: req.Enabled == nil || *req.Enabled})
		case r.URL.Path == "/api/v1/build-cache/stats":
			_ = json.NewEncoder(w).Encode(apiclient.BuildCacheStats{Prefix: "build-cache/web/", Objects: 4, Bytes: 99})
		case r.URL.Path == "/api/v1/build-cache/clear":
			_ = json.NewEncoder(w).Encode(apiclient.BuildCacheClearResult{Deleted: 4, More: true})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestRun_AppsBuildCache(t *testing.T) {
	srv, calls := newBuildCacheCLIServer(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"show app", []string{"apps", "build-cache", "show", "web"}, "cache export failed"},
		{"show shows bucket usage", []string{"apps", "build-cache", "show", "web"}, "4 objects, 99 bytes"},
		{"show unconfigured", []string{"apps", "build-cache", "show", "other"}, "no build cache setting for other"},
		{"show global unconfigured", []string{"apps", "build-cache", "show", "--global"}, "no build cache setting for all apps"},
		{"set app", []string{"apps", "build-cache", "set", "web", "--target", "bkt_1", "--mode", "min"}, "mode min"},
		{"set global", []string{"apps", "build-cache", "set", "--global", "--target", "bkt_1"}, "all apps set"},
		{"clear", []string{"apps", "build-cache", "clear", "web"}, "run the command again"},
		{"remove", []string{"apps", "build-cache", "remove", "web"}, "removed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout %q missing %q", stdout, tt.want)
			}
		})
	}
	if len(*calls) == 0 {
		t.Fatal("server saw no calls")
	}
}

func TestRun_AppsBuildCacheValidation(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{
		{"apps", "build-cache", "set", "web", "--api-url", "http://127.0.0.1:1"},
		{"apps", "build-cache", "set", "--target", "bkt_1", "--api-url", "http://127.0.0.1:1"},
		{"apps", "build-cache", "clear", "--api-url", "http://127.0.0.1:1"},
		{"apps", "build-cache", "show", "--api-url", "http://127.0.0.1:1"},
	} {
		if got := run("levelrail-cli-test", args, &out, &errb, envMap()); got != exitValidation {
			t.Fatalf("%v exit = %d, want %d", args, got, exitValidation)
		}
	}
}
