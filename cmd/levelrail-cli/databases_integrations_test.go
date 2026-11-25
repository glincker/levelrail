package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_DatabasesIntegrationsList_JSON(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DatabaseIntegrationsResource{
			Database: "main", Engine: "postgres",
			Integrations: []apiclient.DatabaseIntegrationCard{{ID: "datadog", Category: "monitor", State: apiclient.DatabaseIntegrationState{Code: "available"}}},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "integrations", "list", "main", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d stderr=%q", got, stderr.String())
	}
	var res apiclient.DatabaseIntegrationsResource
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil || len(res.Integrations) != 1 {
		t.Fatalf("json = %q err = %v", stdout.String(), err)
	}
	if gotPath != "/api/v1/databases/main/integrations" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestRun_DatabasesIntegrations_NeedsDatabase(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"databases", "integrations", "list", "--api-url", "http://127.0.0.1:1"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Errorf("exit = %d", got)
	}
}
