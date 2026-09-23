package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeHealthServer serves GET/PUT/DELETE /api/v1/apps/web/health from an in-memory value.
func fakeHealthServer(t *testing.T, initial *serviceHealth) (*httptest.Server, *[]string, **serviceHealth) {
	t.Helper()
	current := initial
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodPut:
			var h serviceHealth
			_ = json.NewDecoder(r.Body).Decode(&h)
			current = &h
		case http.MethodDelete:
			current = nil
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appHealthResource{Name: "web", Health: current})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &current
}

func TestRun_AppsHealth_Set(t *testing.T) {
	existingLiveness := &serviceProbe{Path: "/livez"}
	tests := []struct {
		name      string
		args      []string
		check     func(t *testing.T, h *serviceHealth)
		wantInOut string
	}{
		{
			name: "https readiness keeps liveness",
			args: []string{"--probe", "readiness", "--path", "/health", "--scheme", "https", "--tls-skip-verify", "--follow-redirects", "false", "--expected-status", "200-399", "--timeout", "3s"},
			check: func(t *testing.T, h *serviceHealth) {
				r := h.Readiness
				if r == nil || r.Scheme != "https" || !r.TLSSkipVerify || r.FollowRedirects == nil || *r.FollowRedirects || r.ExpectedStatus != "200-399" || r.Timeout != 3e9 {
					t.Errorf("readiness = %+v", r)
				}
				if h.Liveness == nil || h.Liveness.Path != "/livez" {
					t.Errorf("liveness = %+v, want the existing probe kept", h.Liveness)
				}
			},
			wantInOut: "readiness: GET https://:port/health (expect 200-399, redirects not followed, TLS verify off), timeout 3s",
		},
		{
			name: "exec liveness",
			args: []string{"--probe", "liveness", "--exec", "pg_isready -U app", "--failures", "5"},
			check: func(t *testing.T, h *serviceHealth) {
				l := h.Liveness
				if l == nil || strings.Join(l.Exec, "|") != "/bin/sh|-c|pg_isready -U app" || l.Failures != 5 {
					t.Errorf("liveness = %+v", l)
				}
			},
			wantInOut: `liveness: exec "pg_isready -U app", 5 failures`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, calls, current := fakeHealthServer(t, &serviceHealth{Liveness: existingLiveness})
			args := append([]string{"apps", "health", "set", "web", "--api-url", srv.URL}, tt.args...)
			stdout, _ := runCLIExpectOK(t, args)
			if got := strings.Join(*calls, ","); got != "GET /api/v1/apps/web/health,PUT /api/v1/apps/web/health" {
				t.Errorf("calls = %s", got)
			}
			tt.check(t, *current)
			if !strings.Contains(stdout, tt.wantInOut) {
				t.Errorf("stdout = %q, want containing %q", stdout, tt.wantInOut)
			}
		})
	}
}

func TestRun_AppsHealth_Set_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		want    int
	}{
		{name: "missing probe", args: []string{"--path", "/"}, wantErr: "requires --probe readiness", want: exitUsage},
		{name: "skip verify on http", args: []string{"--probe", "readiness", "--path", "/", "--tls-skip-verify"}, wantErr: "tls_skip_verify only applies to scheme: https"},
		{name: "path and exec", args: []string{"--probe", "readiness", "--path", "/", "--exec", "true"}, wantErr: "not both"},
		{name: "bad follow", args: []string{"--probe", "readiness", "--path", "/", "--follow-redirects", "maybe"}, wantErr: "must be true or false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"apps", "health", "set", "web", "--api-url", "http://unused"}, tt.args...)
			got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if got == exitOK || (tt.want != 0 && got != tt.want) {
				t.Fatalf("exit = %d, want failure %d", got, tt.want)
			}
			if !strings.Contains(stderr.String()+stdout.String(), tt.wantErr) {
				t.Errorf("output = %q, want containing %q", stderr.String()+stdout.String(), tt.wantErr)
			}
		})
	}
}

func TestRun_AppsHealth_GetAndClear(t *testing.T) {
	srv, calls, current := fakeHealthServer(t, &serviceHealth{Readiness: &serviceProbe{Path: "/ready"}, Liveness: &serviceProbe{Path: "/live"}})

	stdout, _ := runCLIExpectOK(t, []string{"apps", "health", "get", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "readiness: GET http://:port/ready (expect 200-299)") {
		t.Errorf("get stdout = %q", stdout)
	}

	runCLIExpectOK(t, []string{"apps", "health", "clear", "web", "--probe", "liveness", "--api-url", srv.URL})
	if h := *current; h == nil || h.Liveness != nil || h.Readiness == nil {
		t.Errorf("after clearing liveness = %+v", h)
	}

	stdout, _ = runCLIExpectOK(t, []string{"apps", "health", "clear", "web", "--api-url", srv.URL})
	if *current != nil || !strings.Contains(stdout, "no probes configured") {
		t.Errorf("after clear all: health = %+v stdout = %q", *current, stdout)
	}
	if last := (*calls)[len(*calls)-1]; last != "DELETE /api/v1/apps/web/health" {
		t.Errorf("last call = %s", last)
	}
}
