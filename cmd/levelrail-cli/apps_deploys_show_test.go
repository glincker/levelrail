package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsDeploysShow(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployAttemptResource{
			ID: "dep_2", ServiceName: "web", Status: "failed", Image: "web:2", StartedAt: time.Now(),
			Failure: &apiclient.DeployFailure{Code: "missing_env", Cause: "A required variable has no value.", SuggestedFix: "Set API_KEY.", DocsURL: "/deploy-failures#missing_env", LogExcerpt: "line one\nline two", DeployID: "dep_2", App: "web"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "show", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploys/latest" {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{"missing_env", "Set API_KEY.", "/deploy-failures#missing_env", "line two"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}

	jsonOut, _ := runCLIExpectOK(t, []string{"apps", "deploys", "show", "web", "dep_2", "--json", "--api-url", srv.URL})
	var got deployAttemptResource
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil || got.Failure == nil || got.Failure.Code != "missing_env" {
		t.Fatalf("json output = %q (%v)", jsonOut, err)
	}
	if gotPath != "/api/v1/apps/web/deploys/dep_2" {
		t.Fatalf("path = %q", gotPath)
	}
}
