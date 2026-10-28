package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Returns one generic JSON body for every path, so both template calls decode without per-path routing.
func newMultiPathEchoServer(t *testing.T, gotPaths *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPaths = append(*gotPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/compose") {
			_ = json.NewEncoder(w).Encode(composeDeployResult{AppID: "postgres"})
			return
		}
		_ = json.NewEncoder(w).Encode(serviceTemplateDetail{ID: "postgres", Name: "Postgres", Compose: "services:\n  db:\n    image: postgres\n"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_Templates_List(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []serviceTemplateListItem{
		{ID: "postgres", Name: "Postgres", Category: "database", Slogan: "Relational database"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "list", "--api-url", srv.URL})

	if gotPath != "/api/v1/service-templates" {
		t.Errorf("path = %s, want /api/v1/service-templates", gotPath)
	}
	if !strings.Contains(stdout, "postgres") || !strings.Contains(stdout, "Postgres") {
		t.Errorf("stdout = %q, want the postgres entry listed", stdout)
	}
}

func TestRun_Templates_List_RecommendedMemory(t *testing.T) {
	srv := newListEchoServer(t, nil, []serviceTemplateListItem{
		{ID: "ollama-mistral", Name: "Ollama: Mistral 7B", Category: "AI", Slogan: "Mistral locally", RecommendedMemoryBytes: 6 * 1024 * 1024 * 1024},
		{ID: "n8n", Name: "n8n", Category: "Automation", Slogan: "Automations"},
	})

	stdout, _ := runCLIExpectOK(t, []string{"templates", "list", "--api-url", srv.URL})

	if !strings.Contains(stdout, "~6 GiB") {
		t.Errorf("stdout = %q, want the ollama-mistral row to show ~6 GiB", stdout)
	}
	var n8nLine string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "n8n") {
			n8nLine = line
		}
	}
	if n8nLine == "" {
		t.Fatalf("stdout = %q, want an n8n row", stdout)
	}
	if strings.Contains(n8nLine, "GiB") {
		t.Errorf("n8n row = %q, want no RAM figure since it declares none", n8nLine)
	}
}

func TestRun_Templates_Get_RecommendedMemory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serviceTemplateDetail{
			ID: "ollama-mistral", Name: "Ollama: Mistral 7B", RecommendedMemoryBytes: 6 * 1024 * 1024 * 1024,
			Compose: "services:\n  ollama:\n    image: ollama/ollama:0.33.3\n",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "get", "ollama-mistral", "--api-url", srv.URL})

	if !strings.Contains(stdout, "recommended_ram:   ~6 GiB") {
		t.Errorf("stdout = %q, want a recommended_ram line", stdout)
	}
}

func TestRun_Templates_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []serviceTemplateListItem{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no templates") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_Templates_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, serviceTemplateDetail{ID: "postgres", Name: "Postgres", Compose: "services:\n  db:\n    image: postgres\n"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "get", "postgres", "--api-url", srv.URL})

	if gotPath != "/api/v1/service-templates/postgres" {
		t.Errorf("path = %s, want /api/v1/service-templates/postgres", gotPath)
	}
	if !strings.Contains(stdout, "image: postgres") {
		t.Errorf("stdout = %q, want the compose body included", stdout)
	}
}

func TestRun_Templates_Show(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, serviceTemplateDetail{ID: "agent", Name: "Agent", Requirements: apiclient.TemplateRequirements{
		Level:       "elevated",
		Findings:    []apiclient.TemplateFinding{{Code: "docker_socket", Severity: "elevated", Message: "Mounts the Docker socket", Services: []string{"agent"}}},
		Protections: []apiclient.TemplateProtection{{Code: "rollback", Label: "One-step rollback"}},
	}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "show", "agent", "--api-url", srv.URL})
	for _, want := range []string{"level: elevated", "[elevated] Mounts the Docker socket (agent)", "One-step rollback"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}

	jsonOut, _ := runCLIExpectOK(t, []string{"templates", "show", "agent", "--json", "--api-url", srv.URL})
	if !strings.Contains(jsonOut, `"level": "elevated"`) && !strings.Contains(jsonOut, `"level":"elevated"`) {
		t.Errorf("json stdout = %q", jsonOut)
	}
}

func TestRun_Templates_Get_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"template not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"templates", "get", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "template not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_Templates_Deploy(t *testing.T) {
	var gotPaths []string
	server := newMultiPathEchoServer(t, &gotPaths)

	stdout, _ := runCLIExpectOK(t, []string{"templates", "deploy", "postgres", "--api-url", server.URL})

	if len(gotPaths) != 2 {
		t.Fatalf("got %d requests, want 2 (get template, then deploy compose): %v", len(gotPaths), gotPaths)
	}
	if gotPaths[0] != "/api/v1/service-templates/postgres" {
		t.Errorf("first request path = %s, want /api/v1/service-templates/postgres", gotPaths[0])
	}
	if gotPaths[1] != "/api/v1/apps/postgres/compose" {
		t.Errorf("second request path = %s, want /api/v1/apps/postgres/compose", gotPaths[1])
	}
	if !strings.Contains(stdout, "app_id") {
		t.Errorf("stdout = %q, want the deploy result", stdout)
	}
}

func TestRun_Templates_Deploy_CustomName(t *testing.T) {
	var gotPaths []string
	server := newMultiPathEchoServer(t, &gotPaths)

	runCLIExpectOK(t, []string{"templates", "deploy", "postgres", "--name", "my-db", "--api-url", server.URL})

	if len(gotPaths) != 2 || gotPaths[1] != "/api/v1/apps/my-db/compose" {
		t.Errorf("requests = %v, want the second one against /api/v1/apps/my-db/compose", gotPaths)
	}
}

func TestRun_Templates_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"templates", "-h"})
	if !strings.Contains(stdout, "templates list") || !strings.Contains(stdout, "templates deploy") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_Templates_List_VerifiedFilter(t *testing.T) {
	srv := newListEchoServer(t, nil, []serviceTemplateListItem{
		{ID: "grafana", Name: "Grafana", Category: "Monitoring", Slogan: "Dashboards", Verified: true},
		{ID: "untested-thing", Name: "Untested", Category: "Misc", Slogan: "Maybe"},
	})

	all, _ := runCLIExpectOK(t, []string{"templates", "list", "--api-url", srv.URL})
	if !strings.Contains(all, "VERIFIED") || !strings.Contains(all, "untested-thing") {
		t.Errorf("unfiltered list = %q, want VERIFIED column and both rows", all)
	}
	only, _ := runCLIExpectOK(t, []string{"templates", "list", "--verified", "--api-url", srv.URL})
	if !strings.Contains(only, "grafana") || strings.Contains(only, "untested-thing") {
		t.Errorf("--verified list = %q, want only grafana", only)
	}
}

func TestRun_Templates_Get_Description(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serviceTemplateDetail{ID: "rustdesk-server", Name: "RustDesk", Slogan: "Short", Description: "Set RELAY.\nOpen TCP 21117.", Compose: "services: {}\n"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"templates", "get", "rustdesk-server", "--api-url", srv.URL})
	if !strings.Contains(stdout, "description:\n  Set RELAY.\n  Open TCP 21117.\n") {
		t.Errorf("stdout = %q, want an indented description block", stdout)
	}
}
