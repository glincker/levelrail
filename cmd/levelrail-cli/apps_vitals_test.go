package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsVitals_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/vitals" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		st := map[string]apiclient.AppVitalsStat{"LCP": {Count: 4, P75: 2000, Rating: "good"}, "CLS": {Count: 4, P75: 0.15, Rating: "needs-improvement"}}
		_ = json.NewEncoder(w).Encode(apiclient.AppVitalsResource{Enabled: true, Days: 7, Overall: st, Routes: []apiclient.AppVitalsRoute{{Route: "/", Metrics: st}}})
	}))
	t.Cleanup(srv.Close)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "vitals", "web", "--api-url", srv.URL})
	for _, want := range []string{"2000ms good", "0.15 needs-improvement", "all routes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want containing %q", stdout, want)
		}
	}
}
