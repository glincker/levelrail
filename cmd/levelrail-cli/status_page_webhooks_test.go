package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestStatusPageWebhooksCommands(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusCreated, apiclient.StatusWebhook{ID: "swh_1", SigningSecret: "whsec_demo"}) //nolint:gosec // fixture value
	stdout, _ := runCLIExpectOK(t, []string{"status-page", "webhooks", "add", "--url", "https://example.com/hook", "--event", "incident_opened", "--api-url", srv.URL})
	if rec.method != http.MethodPost || rec.path != "/api/v1/status-page/webhooks" || !strings.Contains(rec.body, `"incident_opened"`) {
		t.Errorf("add call = %+v", rec)
	}
	if !strings.Contains(stdout, "whsec_demo") {
		t.Errorf("add output should show the secret once: %q", stdout)
	}
	runCLIExpectValidationError(t, []string{"status-page", "webhooks", "add", "--api-url", srv.URL})

	srv2, rec2 := recordingServer(t, http.StatusOK, apiclient.StatusWebhookTest{Delivered: true, StatusCode: 200})
	runCLIExpectOK(t, []string{"status-page", "webhooks", "test", "swh_1", "--api-url", srv2.URL})
	if rec2.path != "/api/v1/status-page/webhooks/swh_1/test" {
		t.Errorf("test call = %+v", rec2)
	}
	srv3, rec3 := recordingServer(t, http.StatusOK, apiclient.StatusWebhook{ID: "swh_1", Paused: true})
	runCLIExpectOK(t, []string{"status-page", "webhooks", "pause", "swh_1", "--api-url", srv3.URL})
	if rec3.path != "/api/v1/status-page/webhooks/swh_1/pause" {
		t.Errorf("pause call = %+v", rec3)
	}
}
