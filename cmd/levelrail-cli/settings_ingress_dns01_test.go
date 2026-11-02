package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsIngressDNS01_ShowEnableDisable(t *testing.T) {
	var method, path string
	var body map[string]bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, body = r.Method, r.URL.Path, nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"source":"stored","provider_connected":true}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ingress", "dns01", "show", "--api-url", srv.URL})
	if method != http.MethodGet || path != "/api/v1/settings/ingress/dns01" || !strings.Contains(stdout, "on (source: stored") {
		t.Errorf("show: %s %s %q", method, path, stdout)
	}
	runCLIExpectOK(t, []string{"settings", "ingress", "dns01", "enable", "--api-url", srv.URL})
	if method != http.MethodPut || !body["enabled"] {
		t.Errorf("enable: %s %v", method, body)
	}
	runCLIExpectOK(t, []string{"settings", "ingress", "dns01", "disable", "--api-url", srv.URL})
	if v, ok := body["enabled"]; !ok || v {
		t.Errorf("disable body = %v", body)
	}
}
