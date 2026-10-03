package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeHealthScoreServer(t *testing.T, resource appHealthScoreResource) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/health-score" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resource)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsHealthScore_Human(t *testing.T) {
	resource := appHealthScoreResource{
		AppName: "web",
		Status:  "fail",
		Categories: []appHealthScoreCategory{
			{Key: "deploy", Label: "Deploy health", Status: "pass", Reason: "last 3 deploy attempt(s) all succeeded"},
			{Key: "security", Label: "Security", Status: "fail", Reason: "missing required secret(s): API_KEY"},
		},
	}
	srv := fakeHealthScoreServer(t, resource)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "health-score", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "web: fail") {
		t.Errorf("stdout = %q, want containing %q", stdout, "web: fail")
	}
	if !strings.Contains(stdout, "Deploy health") || !strings.Contains(stdout, "pass") {
		t.Errorf("stdout = %q, want the deploy category rendered", stdout)
	}
	if !strings.Contains(stdout, "missing required secret(s): API_KEY") {
		t.Errorf("stdout = %q, want the security category's reason", stdout)
	}
}

func TestRun_AppsHealthScore_JSON(t *testing.T) {
	resource := appHealthScoreResource{
		AppName:    "web",
		Status:     "pass",
		Categories: []appHealthScoreCategory{{Key: "deploy", Label: "Deploy health", Status: "pass", Reason: "all good"}},
	}
	srv := fakeHealthScoreServer(t, resource)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "health-score", "web", "--api-url", srv.URL, "--json"})

	var got appHealthScoreResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal stdout %q: %v", stdout, err)
	}
	if got.AppName != "web" || got.Status != "pass" || len(got.Categories) != 1 {
		t.Errorf("got = %+v", got)
	}
}

func TestRun_AppsHealthScore_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "health-score", "missing", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want failure", got)
	}
	if !strings.Contains(stdout.String()+stderr.String(), "app not found") {
		t.Errorf("output = %q, want containing %q", stdout.String()+stderr.String(), "app not found")
	}
}
