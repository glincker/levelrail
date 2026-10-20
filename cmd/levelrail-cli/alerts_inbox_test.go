package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestAlertsInboxList(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusOK, []apiclient.InboxItem{{AlertHistoryEntry: apiclient.AlertHistoryEntry{ID: "ah_1", RuleName: "cpu", Severity: "critical", Event: "fired"}}})
	stdout, _ := runCLIExpectOK(t, []string{"alerts", "inbox", "list", "--unread", "--limit", "5", "--api-url", srv.URL})
	if rec.method != http.MethodGet || rec.path != "/api/v1/inbox" || !strings.Contains(rec.query, "unread=true") {
		t.Fatalf("call = %+v", rec)
	}
	if !strings.Contains(stdout, "ah_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestAlertsInboxReadNeedsTarget(t *testing.T) {
	srv, _ := recordingServer(t, http.StatusNoContent, nil)
	runCLIExpectValidationError(t, []string{"alerts", "inbox", "read", "--api-url", srv.URL})
	runCLIExpectOK(t, []string{"alerts", "inbox", "read", "--all", "--api-url", srv.URL})
}

func TestApplyInboxPrefFlags(t *testing.T) {
	base := apiclient.InboxPrefs{MinSeverity: "warning", MutedApps: []string{"a", "b"}}
	got := applyInboxPrefFlags(base, "critical", "on", "chn_1", 6, []string{"c", "a"}, []string{"b"})
	raw, _ := json.Marshal(got)
	want := `{"min_severity":"critical","muted_apps":["a","c"],"digest_enabled":true,"digest_hour":6,"digest_channel_id":"chn_1"}`
	if string(raw) != want {
		t.Fatalf("got %s want %s", raw, want)
	}
}
