package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsWebhookDeliveriesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []webhookDeliveryResource{
		{ID: "whd_1", ServiceName: "web", Provider: "github", EventType: "push", SignatureValid: true, Matched: true, StatusCode: 200, ReceivedAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "webhook-deliveries", "list", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/webhook-deliveries" {
		t.Errorf("path = %q, want /api/v1/apps/web/webhook-deliveries", gotPath)
	}
	if !strings.Contains(stdout, "whd_1") || !strings.Contains(stdout, "github") {
		t.Errorf("stdout = %q, want the delivery id and provider listed", stdout)
	}
}

func TestRun_AppsWebhookDeliveriesList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []webhookDeliveryResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "webhook-deliveries", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no webhook deliveries") {
		t.Errorf("stdout = %q, want an empty-state message", stdout)
	}
}

func TestRun_AppsWebhookDeliveriesList_LimitAndBeforeFlags(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "webhook-deliveries", "list", "web", "--limit", "5", "--before", "2026-08-01T00:00:00Z", "--api-url", srv.URL})
	if !strings.Contains(gotQuery, "limit=5") || !strings.Contains(gotQuery, "before=2026-08-01T00%3A00%3A00Z") {
		t.Errorf("query = %q, want limit and before params", gotQuery)
	}
}

func TestRun_AppsWebhookDeliveriesReplay(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200,"message":"deploy triggered: web:sha1\n"}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "webhook-deliveries", "replay", "web", "whd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/webhook-deliveries/whd_1/replay" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/webhook-deliveries/whd_1/replay", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "replayed delivery") || !strings.Contains(stdout, "status=200") {
		t.Errorf("stdout = %q, want a replay confirmation with the resulting status", stdout)
	}
}

func TestRun_AppsWebhookDeliveriesReplay_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "webhook-deliveries", "replay", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitUsage, stdout.String(), stderr.String())
	}
}
