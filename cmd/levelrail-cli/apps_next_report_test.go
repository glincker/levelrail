package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func nextReportServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/next-advisor":
			_, _ = w.Write([]byte(`{"detected":true,"findings":[{"id":"web-vitals-off","severity":"info","title":"Web vitals are off","detail":"d","fix":"turn on vitals"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/deploys/dep_1/build-report":
			_, _ = w.Write([]byte(`{"detected":true,"routes":[{"path":"/","kind":"static","size_kb":1.2,"first_load_kb":90.5}],"shared_first_load_kb":87.3,"warnings":["slow page"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestAppsNextAdvisor(t *testing.T) {
	srv := nextReportServer(t)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("cli", []string{"apps", "next-advisor", "web", "--api-url", srv.URL, "--token", "t"}, &out, &errb, envMap())
	if code != exitOK || !strings.Contains(out.String(), "web-vitals-off") || !strings.Contains(out.String(), "turn on vitals") {
		t.Fatalf("table: code %d out %q err %q", code, out.String(), errb.String())
	}
	out.Reset()
	code = run("cli", []string{"apps", "next-advisor", "web", "--json", "--api-url", srv.URL, "--token", "t"}, &out, &errb, envMap())
	if code != exitOK || !strings.Contains(out.String(), `"detected": true`) && !strings.Contains(out.String(), `"detected":true`) {
		t.Fatalf("json: code %d out %q", code, out.String())
	}
	if code := run("cli", []string{"apps", "next-advisor", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitUsage {
		t.Fatalf("missing app: code %d", code)
	}
}

func TestAppsDeploysBuildReport(t *testing.T) {
	srv := nextReportServer(t)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("cli", []string{"apps", "deploys", "build-report", "web", "dep_1", "--api-url", srv.URL, "--token", "t"}, &out, &errb, envMap())
	s := out.String()
	if code != exitOK || !strings.Contains(s, "FIRST_LOAD_KB") || !strings.Contains(s, "static") || !strings.Contains(s, "87.3 kB") || !strings.Contains(s, "warning: slow page") {
		t.Fatalf("table: code %d out %q err %q", code, s, errb.String())
	}
	if code := run("cli", []string{"apps", "deploys", "build-report", "web", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitUsage {
		t.Fatalf("missing id: code %d", code)
	}
}
