package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsDashboardURL_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, dashboardURLResource{DashboardURL: "https://dash.example.com"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "dashboard-url", "get", "--api-url", srv.URL})
	if gotPath != "/api/v1/settings/dashboard-url" {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.Contains(stdout, "dashboard_url: https://dash.example.com") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_SettingsDashboardURL_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody dashboardURLResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "dashboard-url", "set", "--url", "https://dash.example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/dashboard-url" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody.DashboardURL != "https://dash.example.com" {
		t.Errorf("body = %+v", gotBody)
	}
	if !strings.Contains(stdout, "dashboard_url: https://dash.example.com") {
		t.Errorf("stdout = %q", stdout)
	}
}
