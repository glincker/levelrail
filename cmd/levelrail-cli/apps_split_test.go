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

func TestRun_AppsSplit(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		_ = json.NewEncoder(w).Encode(apiclient.SplitTest{Active: true, Supported: true, TestID: "t1", Status: "running", Variants: []apiclient.SplitVariant{{ID: "t1-p", Label: "Production", Weight: 70, Status: "live"}}})
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		args               []string
		method, path, body string
	}{
		{[]string{"start", "web", "--variant", "branch:redesign=30"}, http.MethodPost, "/api/v1/apps/web/split", `{"variants":[{"kind":"branch","ref":"redesign","weight":30}]}`},
		{[]string{"start", "web", "--variant", "branch:a=10", "--variant", "release:web-r1=5", "--cookie-name", "ab", "--cookie-ttl", "24h"}, http.MethodPost, "/api/v1/apps/web/split", `{"variants":[{"kind":"branch","ref":"a","weight":10},{"kind":"release","ref":"web-r1","weight":5}],"cookie_name":"ab","cookie_ttl":"24h"}`},
		{[]string{"weights", "web", "--set", "production=60", "--set", "redesign=40"}, http.MethodPut, "/api/v1/apps/web/split", `{"weights":{"production":60,"redesign":40}}`},
		{[]string{"status", "web"}, http.MethodGet, "/api/v1/apps/web/split", ""},
		{[]string{"promote", "web", "--variant", "redesign"}, http.MethodPost, "/api/v1/apps/web/split/promote", `{"variant":"redesign"}`},
		{[]string{"stop", "web"}, http.MethodDelete, "/api/v1/apps/web/split", ""},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"apps", "split"}, tc.args...)
		args = append(args, "--token", "t", "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v: exit = %d (stderr=%q)", tc.args, got, stderr.String())
		}
		if gotMethod != tc.method || gotPath != tc.path || strings.TrimSpace(gotBody) != tc.body {
			t.Errorf("%v: %s %s %q, want %s %s %q", tc.args, gotMethod, gotPath, gotBody, tc.method, tc.path, tc.body)
		}
	}
}

func TestRun_AppsSplitUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"start", "web"},
		{"start", "web", "--variant", "redesign=30"},
		{"weights", "web"},
		{"weights", "web", "--set", "oops"},
		{"promote", "web"},
		{"nope"},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"apps", "split"}, args...)
		full = append(full, "--token", "t", "--api-url", "http://127.0.0.1:1")
		if got := run("levelrail-cli-test", full, &stdout, &stderr, envMap()); got != exitUsage {
			t.Errorf("%v: exit = %d, want usage", args, got)
		}
	}
}
