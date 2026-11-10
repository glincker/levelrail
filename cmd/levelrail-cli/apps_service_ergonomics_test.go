package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParseScalePairs(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []scalePair
		wantErr string
	}{
		{"two services", []string{"web=2", "worker=1"}, []scalePair{{"web", 2}, {"worker", 1}}, ""},
		{"missing equals", []string{"web"}, nil, "not service=count"},
		{"empty count", []string{"web="}, nil, "not service=count"},
		{"empty key", []string{"=2"}, nil, "not service=count"},
		{"not a number", []string{"web=two"}, nil, "whole number"},
		{"zero", []string{"web=0"}, nil, "at least 1"},
		{"negative", []string{"web=-1"}, nil, "at least 1"},
		{"duplicate", []string{"web=1", "web=2"}, nil, "given twice"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScalePairs(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || len(got) != len(tc.want) {
				t.Fatalf("got %v, %v, want %v", got, err, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("pair %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func scaleFake(t *testing.T) (*httptest.Server, func() map[string]float64) {
	t.Helper()
	var mu sync.Mutex
	puts := map[string]float64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/shop/group":
			_, _ = w.Write([]byte(`{"services":[{"name":"shop-web","image":"i","replicas":1},{"name":"shop-worker","image":"i","replicas":1}],"status":{}}`))
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			mu.Lock()
			puts[strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")], _ = m["replicas"].(float64)
			mu.Unlock()
			_, _ = w.Write(body)
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": strings.TrimPrefix(r.URL.Path, "/api/v1/apps/"), "image": "i", "replicas": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() map[string]float64 { mu.Lock(); defer mu.Unlock(); return puts }
}

func TestAppsScale_ServicePairs(t *testing.T) {
	srv, puts := scaleFake(t)
	runCLIExpectOK(t, []string{"apps", "scale", "shop", "web=3", "worker=2", "--api-url", srv.URL, "--token", "t"})
	if got := puts(); got["shop-web"] != 3 || got["shop-worker"] != 2 {
		t.Errorf("puts = %v, want shop-web 3 and shop-worker 2", got)
	}
}

func TestAppsScale_ServicePairErrorsChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown service lists the real ones", []string{"web=2", "cache=1"}, "services: web, worker"},
		{"bad pair", []string{"web=2", "worker"}, "not service=count"},
		{"mixed with replicas", []string{"web=2", "--replicas", "3"}, "cannot be combined"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, puts := scaleFake(t)
			var out, errb bytes.Buffer
			args := append([]string{"apps", "scale", "shop"}, tc.args...)
			args = append(args, "--api-url", srv.URL, "--token", "t")
			if got := run("levelrail-cli-test", args, &out, &errb, envMap()); got != exitValidation {
				t.Fatalf("exit = %d, want %d (%s)", got, exitValidation, errb.String())
			}
			if !strings.Contains(errb.String(), tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errb.String(), tc.want)
			}
			if len(puts()) != 0 {
				t.Errorf("puts = %v, want none", puts())
			}
		})
	}
}

func TestAppsRestart_ServiceTargetsSibling(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"name":"shop-web","image":"i"}`))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "restart", "shop", "--service", "web", "--api-url", srv.URL, "--token", "t"})
	if path != "/api/v1/apps/shop-web/restart" {
		t.Errorf("path = %q, want /api/v1/apps/shop-web/restart", path)
	}
}

func TestAppsLogs_TerminalFollowsByDefault(t *testing.T) {
	tests := []struct {
		name     string
		tty      bool
		args     []string
		wantPath string
	}{
		{"terminal follows", true, nil, "/api/v1/apps/shop-web/logs/stream"},
		{"terminal with --since searches", true, []string{"--since", "10m"}, "/api/v1/apps/shop-web/logs"},
		{"terminal with -n searches", true, []string{"-n", "5"}, "/api/v1/apps/shop-web/logs"},
		{"terminal with --no-follow searches", true, []string{"--no-follow"}, "/api/v1/apps/shop-web/logs"},
		{"terminal with --json searches", true, []string{"--json"}, "/api/v1/apps/shop-web/logs"},
		{"pipe searches", false, nil, "/api/v1/apps/shop-web/logs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := stdoutIsTerminal
			stdoutIsTerminal = func(io.Writer) bool { return tc.tty }
			t.Cleanup(func() { stdoutIsTerminal = prev })
			var path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/apps/shop-web/logs" || r.URL.Path == "/api/v1/apps/shop-web/logs/stream" {
					path = r.URL.Path
				}
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"entries":[]}`))
			}))
			defer srv.Close()
			var out, errb bytes.Buffer
			args := append([]string{"apps", "logs", "shop", "--service", "web", "--api-url", srv.URL, "--token", "t"}, tc.args...)
			run("levelrail-cli-test", args, &out, &errb, envMap())
			if path != tc.wantPath {
				t.Errorf("hit %q, want %q (stderr=%q)", path, tc.wantPath, errb.String())
			}
		})
	}
}
