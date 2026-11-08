package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_Templates_Install_DryRun(t *testing.T) {
	var got apiclient.TemplateInstallRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(apiclient.TemplateInstallPlan{
			Name:    "plunk",
			Missing: []string{"AWS_REGION"},
			Env:     []apiclient.TemplateInstallPlanEnv{{Key: "JWT_SECRET", Source: "supplied", Value: "********", Secret: true}},
			Compose: "services: {}\n",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "install", "plunk", "--dry-run", "--env", "JWT_SECRET=hunter2", "--api-url", srv.URL})

	if !got.DryRun {
		t.Errorf("dry_run not sent: %+v", got)
	}
	if !strings.Contains(stdout, "missing required settings: AWS_REGION") || strings.Contains(stdout, "hunter2") {
		t.Errorf("stdout = %q", stdout)
	}
}
