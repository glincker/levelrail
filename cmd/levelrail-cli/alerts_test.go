package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type recordedCall struct {
	method, path, query, body string
}

func recordingServer(t *testing.T, status int, reply any) (*httptest.Server, *recordedCall) {
	t.Helper()
	rec := &recordedCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.method, rec.path, rec.query, rec.body = r.Method, r.URL.Path, r.URL.RawQuery, string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if reply != nil {
			_ = json.NewEncoder(w).Encode(reply)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestAlertsSilencesCreate(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusCreated, apiclient.SilenceResource{ID: "sil_1", Status: "active"})
	stdout, _ := runCLIExpectOK(t, []string{"alerts", "silences", "create", "--app", "web", "--severity", "warning", "--label", "team=core",
		"--for", "4h", "--reason", "deploy", "--api-url", srv.URL})
	if rec.method != http.MethodPost || rec.path != "/api/v1/alert-silences" {
		t.Fatalf("call = %+v", rec)
	}
	var got apiclient.CreateSilenceRequest
	_ = json.Unmarshal([]byte(rec.body), &got)
	if got.Duration != "4h" || got.Reason != "deploy" || got.Matchers.Apps[0] != "web" || got.Matchers.Labels["team"] != "core" || got.Matchers.Severities[0] != "warning" {
		t.Errorf("request = %+v", got)
	}
	if !strings.Contains(stdout, "sil_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestAlertsSilencesCreate_Validation(t *testing.T) {
	srv, _ := recordingServer(t, http.StatusCreated, nil)
	runCLIExpectValidationError(t, []string{"alerts", "silences", "create", "--app", "web", "--api-url", srv.URL})
	runCLIExpectValidationError(t, []string{"alerts", "silences", "create", "--app", "web", "--for", "1h", "--label", "novalue", "--api-url", srv.URL})
}

func TestAlertsSilencesListAndDelete(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusOK, []apiclient.SilenceResource{})
	runCLIExpectOK(t, []string{"alerts", "silences", "list", "--all", "--api-url", srv.URL})
	if rec.path != "/api/v1/alert-silences" || rec.query != "include_expired=true" {
		t.Errorf("list call = %+v", rec)
	}
	srv2, rec2 := recordingServer(t, http.StatusOK, apiclient.SilenceResource{ID: "sil_9"})
	runCLIExpectOK(t, []string{"alerts", "silences", "delete", "sil_9", "--api-url", srv2.URL})
	if rec2.method != http.MethodDelete || rec2.path != "/api/v1/alert-silences/sil_9" {
		t.Errorf("delete call = %+v", rec2)
	}
}

func TestAlertsQuickSilence(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusCreated, apiclient.SilenceResource{ID: "sil_2"})
	runCLIExpectOK(t, []string{"alerts", "silence", "web", "r1", "--for", "24h", "--api-url", srv.URL})
	if rec.path != "/api/v1/apps/web/alerts/r1/silence" || !strings.Contains(rec.body, `"duration":"24h"`) {
		t.Errorf("call = %+v", rec)
	}
}

func TestAlertsMaintenanceCreate(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusCreated, apiclient.MaintenanceWindowResource{ID: "mw_1", Name: "nightly"})
	runCLIExpectOK(t, []string{"alerts", "maintenance", "create", "--name", "nightly", "--cron", "0 3 * * *", "--duration", "2h",
		"--tz", "Europe/Berlin", "--scope", "app", "--target", "web", "--api-url", srv.URL})
	var got apiclient.MaintenanceWindowResource
	_ = json.Unmarshal([]byte(rec.body), &got)
	if rec.path != "/api/v1/alert-maintenance-windows" || got.Timezone != "Europe/Berlin" || got.Scope != "app" || got.Targets[0] != "web" || !got.Enabled {
		t.Errorf("call = %+v body = %+v", rec, got)
	}
	runCLIExpectValidationError(t, []string{"alerts", "maintenance", "create", "--name", "x", "--api-url", srv.URL})
}

func TestAlertsHistoryQuery(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusOK, []apiclient.AlertHistoryEntry{{RuleName: "cpu", Event: "fired", Outcome: "silenced"}})
	stdout, _ := runCLIExpectOK(t, []string{"alerts", "history", "--app", "web", "--outcome", "silenced", "--limit", "5", "--api-url", srv.URL})
	for _, want := range []string{"app=web", "outcome=silenced", "limit=5"} {
		if !strings.Contains(rec.query, want) {
			t.Errorf("query %q missing %q", rec.query, want)
		}
	}
	if !strings.Contains(stdout, "silenced") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestStatusPageCommands(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusOK, apiclient.StatusPageSettings{Title: "Old"})
	runCLIExpectOK(t, []string{"status-page", "set", "--enable", "--title", "Acme", "--api-url", srv.URL})
	if rec.method != http.MethodPut || !strings.Contains(rec.body, `"enabled":true`) || !strings.Contains(rec.body, `"title":"Acme"`) {
		t.Errorf("set call = %+v", rec)
	}
	runCLIExpectValidationError(t, []string{"status-page", "set", "--enable", "--disable", "--api-url", srv.URL})

	srv2, rec2 := recordingServer(t, http.StatusCreated, apiclient.StatusComponent{ID: "sc_1", DisplayName: "Website"})
	runCLIExpectOK(t, []string{"status-page", "components", "add", "--kind", "domain", "--target", "example.com", "--name", "Website", "--api-url", srv2.URL})
	if rec2.path != "/api/v1/status-page/components" || !strings.Contains(rec2.body, `"display_name":"Website"`) {
		t.Errorf("add call = %+v", rec2)
	}
	runCLIExpectValidationError(t, []string{"status-page", "components", "add", "--kind", "domain", "--api-url", srv2.URL})

	srv3, rec3 := recordingServer(t, http.StatusOK, apiclient.StatusIncident{ID: "si_1", Status: "resolved"})
	runCLIExpectOK(t, []string{"status-page", "incidents", "update", "si_1", "--status", "resolved", "--body", "Fixed", "--api-url", srv3.URL})
	if rec3.path != "/api/v1/status-page/incidents/si_1/updates" {
		t.Errorf("update call = %+v", rec3)
	}
}
