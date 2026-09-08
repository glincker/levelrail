package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_ServiceTemplatesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []serviceTemplateListItem{
		{ID: "postgres", Name: "PostgreSQL", Category: "database"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"service-templates", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/service-templates" {
		t.Errorf("path = %q, want /api/v1/service-templates", gotPath)
	}
	if !strings.Contains(stdout, "postgres") || !strings.Contains(stdout, "PostgreSQL") {
		t.Errorf("stdout = %q, want the template listed", stdout)
	}
}

func TestRun_ServiceTemplatesList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]serviceTemplateListItem{{ID: "redis", Name: "Redis", Category: "database"}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"service-templates", "list", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"id": "redis"`) {
		t.Errorf("stdout = %q, want the template as JSON", stdout.String())
	}
}

func TestRun_ServiceTemplatesList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]serviceTemplateListItem{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"service-templates", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no service templates") {
		t.Errorf("stdout = %q, want a no-templates message", stdout.String())
	}
}

func TestRun_ServiceTemplatesGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service-templates/postgres" {
			t.Errorf("path = %q, want /api/v1/service-templates/postgres", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serviceTemplateDetail{ID: "postgres", Name: "PostgreSQL", Compose: "services:\n  db:\n    image: postgres:16\n"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"service-templates", "get", "postgres", "--api-url", srv.URL})
	if !strings.Contains(stdout, "PostgreSQL") || !strings.Contains(stdout, "postgres:16") {
		t.Errorf("stdout = %q, want name and compose body shown", stdout)
	}
}

func TestRun_ServiceTemplatesGet_MissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"service-templates", "get"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
	}
}

func TestRun_ServiceTemplatesGet_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error": "service template not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"service-templates", "get", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "service template not found") {
		t.Errorf("stderr = %q, want the not-found message", stderr)
	}
}

func TestRun_ServiceTemplates_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"service-templates", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
