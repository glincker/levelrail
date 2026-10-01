package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsUpdates_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, updateSettingsResource{Channel: "beta", AutoUpdateEnabled: true})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "updates", "get", "--api-url", srv.URL})
	if gotPath != "/api/v1/updates/settings" {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.Contains(stdout, "channel:             beta") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "auto_update_enabled: true") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_SettingsUpdates_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody updateSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "updates", "set", "--channel", "edge", "--auto-update", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/updates/settings" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody.Channel != "edge" || !gotBody.AutoUpdateEnabled {
		t.Errorf("body = %+v", gotBody)
	}
	if !strings.Contains(stdout, "channel:             edge") {
		t.Errorf("stdout = %q", stdout)
	}
}
