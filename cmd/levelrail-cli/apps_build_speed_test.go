package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsBuildSpeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/apps/web/build-speed" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.BuildSpeed{Builds: 5, WarmKnown: true, ColdP50Seconds: 100, WarmP50Seconds: 10, TimeSavedSeconds: 180,
			Findings: []apiclient.BuildSpeedFinding{{Code: "low_cache_hit", Severity: "info", FixKind: "manual"}}})
	}))
	t.Cleanup(srv.Close)
	out, _ := runCLIExpectOK(t, []string{"apps", "build-speed", "web", "--api-url", srv.URL})
	for _, want := range []string{"cold build p50 1m40s", "low_cache_hit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
