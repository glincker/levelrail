package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func statusReasonsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status-reasons" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.StatusReason{
			{Code: "ReadinessFailed", Kind: "gate", Meaning: "never ready", NextStep: "check the probe"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_StatusReasonsTable(t *testing.T) {
	srv := statusReasonsServer(t)
	var stdout, stderr strings.Builder
	if code := run("levelrail-cli-test", []string{"status-reasons", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d; stderr %s", code, stderr.String())
	}
	for _, want := range []string{"ReadinessFailed (gate)", "never ready", "next: check the probe"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRun_StatusReasonsJSON(t *testing.T) {
	srv := statusReasonsServer(t)
	var stdout, stderr strings.Builder
	if code := run("levelrail-cli-test", []string{"status-reasons", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d; stderr %s", code, stderr.String())
	}
	var got []apiclient.StatusReason
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil || len(got) != 1 || got[0].Code != "ReadinessFailed" {
		t.Fatalf("json output = %q (%v)", stdout.String(), err)
	}
}
