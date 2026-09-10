package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMultiPathEchoServer starts a test server that records every request
// path it sees (in order) and always responds 200 with a minimal,
// generically-shaped JSON body: enough for "templates deploy" to decode
// both its GetServiceTemplate call (a serviceTemplateDetail) and its
// DeployCompose call (a composeDeployResult) without needing to
// distinguish the two requests server-side.
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
