package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_DomainsMaintenanceSet_Options(t *testing.T) {
	var got apiclient.SetDomainMaintenanceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domainMaintenanceResource{
			Domain: "app.example.com", Enabled: true, Title: got.Title, Until: got.Until, WindowID: "mw_1",
			Bypass: []string{"10.0.0.0/8"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{
		"domains", "maintenance", "set", "web", "app.example.com", "--api-url", srv.URL,
		"--title", "Upgrading", "--message", "Back soon", "--retry-after", "10m", "--for", "2h",
		"--bypass", "10.0.0.0/8, 198.51.100.7", "--no-health-bypass",
	}, &stdout, &stderr, envMap())
	if code != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", code, stderr.String())
	}
	if got.Title != "Upgrading" || got.Message != "Back soon" || got.RetryAfterSeconds != 600 ||
		strings.Join(got.Bypass, ",") != "10.0.0.0/8,198.51.100.7" || got.BypassHealthCheck == nil || *got.BypassHealthCheck {
		t.Errorf("request = %+v", got)
	}
	until, err := time.Parse(time.RFC3339, got.Until)
	if err != nil || time.Until(until) < 119*time.Minute || time.Until(until) > 121*time.Minute {
		t.Errorf("until = %q, want about 2h ahead", got.Until)
	}
	for _, want := range []string{"title: Upgrading", "ends: ", "bypass: 10.0.0.0/8"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q: %s", want, stdout.String())
		}
	}
}

func TestRun_DomainsMaintenanceSet_FlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--until", "tomorrow"},
		{"--until", "2030-01-01T00:00:00Z", "--for", "1h"},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"domains", "maintenance", "set", "web", "app.example.com", "--api-url", "http://127.0.0.1:1"}, args...)
		if code := run("levelrail-cli-test", full, &stdout, &stderr, envMap()); code != exitUsage {
			t.Errorf("%v: exit = %d, want usage error (stderr=%q)", args, code, stderr.String())
		}
	}
}
