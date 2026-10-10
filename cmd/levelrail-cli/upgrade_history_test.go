package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_UpgradeHistory_AndAck(t *testing.T) {
	var acked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/updates/history":
			_ = json.NewEncoder(w).Encode(upgradeHistory{
				CurrentVersion: "v0.0.2", Unacknowledged: 1,
				Entries: []upgradeHistoryEntry{{ID: "uh_1", Kind: "upgraded", FromVersion: "v0.0.1", ToVersion: "v0.0.2", Initiator: "unknown", SchemaMoved: true}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/updates/history/uh_1/ack":
			acked = "uh_1"
			_ = json.NewEncoder(w).Encode(upgradeHistoryEntry{ID: "uh_1", FromVersion: "v0.0.1", ToVersion: "v0.0.2", Acknowledged: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"upgrade", "--history", "--api-url", srv.URL})
	for _, want := range []string{"1 unacknowledged", "v0.0.1 -> v0.0.2", "UNACKNOWLEDGED", "schema changed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
	runCLIExpectOK(t, []string{"upgrade", "--ack", "uh_1", "--api-url", srv.URL})
	if acked != "uh_1" {
		t.Errorf("ack not sent, got %q", acked)
	}
}
