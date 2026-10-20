package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_DomainsErrorPagesSet(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com", Pages: []domainErrorPageEntry{{StatusCode: 404, Body: "<h1>not found</h1>"}}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "error-pages", "set", "web", "app.example.com", "--code", "404", "--body", "<h1>not found</h1>", "--api-url", srv.URL})
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/error-pages" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/error-pages", gotPath)
	}
	if !strings.Contains(gotBody, `"status_code":404`) || !strings.Contains(gotBody, "not found") {
		t.Errorf("body = %q, want status_code and body set", gotBody)
	}
	if !strings.Contains(stdout, "404:") {
		t.Errorf("stdout = %q, want the 404 mapping listed", stdout)
	}
}

func TestRun_DomainsErrorPagesSet_BodyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "404.html")
	if err := os.WriteFile(path, []byte("<h1>from file</h1>"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com", Pages: []domainErrorPageEntry{{StatusCode: 404, Body: "<h1>from file</h1>"}}})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"domains", "error-pages", "set", "web", "app.example.com", "--code", "404", "--body-file", path, "--api-url", srv.URL})
	if !strings.Contains(gotBody, "from file") {
		t.Errorf("body = %q, want the file's contents", gotBody)
	}
}

func TestRun_DomainsErrorPagesSet_ConflictingBodyFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "error-pages", "set", "web", "app.example.com", "--code", "404", "--body", "a", "--body-file", "b.html"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want a mutually-exclusive-flags error", stderr.String())
	}
}

func TestRun_DomainsErrorPagesSet_MissingCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "error-pages", "set", "web", "app.example.com", "--body", "a"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--code is required") {
		t.Errorf("stderr = %q, want a missing-code usage error", stderr.String())
	}
}

func TestRun_DomainsErrorPagesSet_MissingBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "error-pages", "set", "web", "app.example.com", "--code", "404"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--body or --body-file is required") {
		t.Errorf("stderr = %q, want a missing-body usage error", stderr.String())
	}
}

func TestRun_DomainsErrorPagesSet_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "error-pages", "set", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}

func TestRun_DomainsErrorPagesGet(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com", Pages: []domainErrorPageEntry{
			{StatusCode: 404, Body: "<h1>not found</h1>"},
			{StatusCode: 503, Body: "<h1>down</h1>"},
		}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "error-pages", "get", "web", "app.example.com", "--api-url", srv.URL})
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/error-pages" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/error-pages", gotPath)
	}
	if !strings.Contains(stdout, "404:") || !strings.Contains(stdout, "503:") {
		t.Errorf("stdout = %q, want both mappings listed", stdout)
	}
}

func TestRun_DomainsErrorPagesGet_FiltersByCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com", Pages: []domainErrorPageEntry{
			{StatusCode: 404, Body: "<h1>not found</h1>"},
			{StatusCode: 503, Body: "<h1>down</h1>"},
		}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "error-pages", "get", "web", "app.example.com", "--code", "404", "--api-url", srv.URL})
	if !strings.Contains(stdout, "404:") {
		t.Errorf("stdout = %q, want the 404 mapping", stdout)
	}
	if strings.Contains(stdout, "503:") {
		t.Errorf("stdout = %q, want the 503 mapping filtered out", stdout)
	}
}

func TestRun_DomainsErrorPagesGet_NoneConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "error-pages", "get", "web", "app.example.com", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no custom error pages configured") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_DomainsErrorPagesClear_OneCode(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "error-pages", "clear", "web", "app.example.com", "--code", "404", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/domains/app.example.com/error-pages" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/error-pages", gotPath)
	}
	if !strings.Contains(stdout, `cleared for domain "app.example.com"`) {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_DomainsErrorPagesClear_AllCodes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(domainErrorPagesResource{Domain: "app.example.com"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"domains", "error-pages", "clear", "web", "app.example.com", "--api-url", srv.URL})
	if gotQuery != "" {
		t.Errorf("query = %q, want empty when --code is omitted (clears every mapping)", gotQuery)
	}
}
