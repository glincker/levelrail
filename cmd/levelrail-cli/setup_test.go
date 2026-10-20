package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func setupGuideFixture() apiclient.SetupGuideResource {
	return apiclient.SetupGuideResource{
		ReadyPercent: 25, Done: 2, Total: 5, NextStep: "first_app",
		Steps: []apiclient.SetupGuideStep{
			{ID: "secure_admin", Title: "Secure the admin account", State: "done", EstimateMinutes: 2, Href: "/settings/security"},
			{ID: "first_app", Title: "Deploy your first app", State: "todo", EstimateMinutes: 5, Href: "/apps"},
			{ID: "app_domain", Title: "Put the app on your domain", State: "todo", EstimateMinutes: 4, Href: "/domains", BlockedBy: []string{"first_app"}},
			{ID: "tls", Title: "Get a valid TLS certificate", State: "not_applicable", Reason: "tls_terminated_upstream", EstimateMinutes: 2, Href: "/settings/certificates"},
			{ID: "teammate", Title: "Invite a teammate", State: "skipped", Optional: true, EstimateMinutes: 2, Href: "/settings/users"},
		},
	}
}

func setupGuideServer(t *testing.T, gotReq *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotReq = r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(setupGuideFixture())
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_SetupStatus(t *testing.T) {
	var got string
	srv := setupGuideServer(t, &got)
	stdout, _ := runCLIExpectOK(t, []string{"setup", "status", "--api-url", srv.URL})
	if got != "GET /api/v1/setup/guide" {
		t.Fatalf("request = %q", got)
	}
	for _, want := range []string{"Ready for production: 25%", "Deploy your first app", "after first_app", "your proxy terminates TLS", "Invite a teammate (optional)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_SetupNext(t *testing.T) {
	var got string
	srv := setupGuideServer(t, &got)
	stdout, _ := runCLIExpectOK(t, []string{"setup", "next", "--api-url", srv.URL})
	if !strings.Contains(stdout, "Next: Deploy your first app (~5 min)") || !strings.Contains(stdout, "Open: /apps") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_SetupSkipAndReset(t *testing.T) {
	var got string
	srv := setupGuideServer(t, &got)
	runCLIExpectOK(t, []string{"setup", "skip", "teammate", "--api-url", srv.URL})
	if got != "PUT /api/v1/setup/guide/steps/teammate" {
		t.Fatalf("skip request = %q", got)
	}
	runCLIExpectOK(t, []string{"setup", "undo", "teammate", "--api-url", srv.URL})
	if got != "PUT /api/v1/setup/guide/steps/teammate" {
		t.Fatalf("undo request = %q", got)
	}
	runCLIExpectOK(t, []string{"setup", "reset", "--api-url", srv.URL})
	if got != "POST /api/v1/setup/guide/reset" {
		t.Fatalf("reset request = %q", got)
	}
}
