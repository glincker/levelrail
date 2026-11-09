package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestMonitorsRotateToken_PrintsNewPingOnce(t *testing.T) {
	srv, rec := recordingServer(t, http.StatusOK, apiclient.Monitor{ID: "mon_1", Kind: "heartbeat", PingPath: "/ping/newtoken"})
	stdout, _ := runCLIExpectOK(t, []string{"monitors", "rotate-token", "mon_1", "--api-url", srv.URL})
	if rec.method != http.MethodPost || rec.path != "/api/v1/monitors/mon_1/rotate-token" {
		t.Fatalf("call = %+v", rec)
	}
	if strings.Count(stdout, "/ping/newtoken") != 1 || !strings.Contains(stdout, "token rotated") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestMonitorsRotateToken_NotFoundFails(t *testing.T) {
	srv, _ := recordingServer(t, http.StatusNotFound, map[string]string{"error": "monitor not found"})
	runCLIExpectAPIError(t, []string{"monitors", "rotate-token", "mon_x", "--api-url", srv.URL})
}
