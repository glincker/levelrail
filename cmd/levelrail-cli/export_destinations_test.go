package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type exportCall struct {
	method string
	path   string
	body   string
}

func newExportServer(t *testing.T, calls *[]exportCall, respond func(r *http.Request) (int, any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*calls = append(*calls, exportCall{r.Method, r.URL.RequestURI(), string(b)})
		status, payload := respond(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if payload != nil {
			_ = json.NewEncoder(w).Encode(payload)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_ExportDestinationsList(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(*http.Request) (int, any) {
		return 200, []apiclient.ExportDestinationResource{{
			ID: "exp_1", Name: "loki", Kind: "logs_loki", Enabled: true,
			Status: apiclient.ExportStatus{SentTotal: 42, QueueDepth: 3, LastError: "boom"},
		}}
	})
	stdout, _ := runCLIExpectOK(t, []string{"export", "destinations", "list", "--api-url", srv.URL})
	if calls[0].path != "/api/v1/export/destinations" || calls[0].method != http.MethodGet {
		t.Fatalf("call = %+v", calls[0])
	}
	for _, want := range []string{"exp_1", "loki", "logs_loki", "42", "boom"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q missing %q", stdout, want)
		}
	}
}

func TestRun_ExportDestinationsCreateSendsSecretsInBodyOnly(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(*http.Request) (int, any) {
		return 201, apiclient.ExportDestinationResource{ID: "exp_2", Name: "audit", Kind: "audit_webhook", HeaderNames: []string{"X-Token"}, HasHMACKey: true}
	})
	stdout, _ := runCLIExpectOK(t, []string{
		"export", "destinations", "create", "--name", "audit", "--kind", "audit_webhook",
		"--endpoint", "https://203.0.113.5/hook", "--header", "X-Token=s3cret", "--hmac-key", "0123456789abcdef",
		"--app", "web,api", "--backfill", "--api-url", srv.URL,
	})
	var req apiclient.CreateExportDestinationRequest
	if err := json.Unmarshal([]byte(calls[0].body), &req); err != nil {
		t.Fatal(err)
	}
	if req.Headers["X-Token"] != "s3cret" || req.HMACKey != "0123456789abcdef" || len(req.Apps) != 2 || !req.Options.Backfill || req.Enabled == nil || !*req.Enabled {
		t.Fatalf("request = %+v", req)
	}
	if strings.Contains(stdout, "s3cret") || strings.Contains(stdout, "0123456789abcdef") {
		t.Fatalf("output leaked a secret: %q", stdout)
	}
}

func TestRun_ExportDestinationsCreateRequiresNameAndKind(t *testing.T) {
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"export", "destinations", "create", "--name", "x", "--api-url", "http://127.0.0.1:1"}, &out, &errb, envMap())
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d", code, exitValidation)
	}
}

func TestRun_ExportDestinationsTestReportsFailureWithNonZeroExit(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(*http.Request) (int, any) {
		return 200, apiclient.ExportTestResult{OK: false, Error: "status 503"}
	})
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"export", "destinations", "test", "exp_1", "--api-url", srv.URL}, &out, &errb, envMap())
	if code == exitOK || !strings.Contains(out.String(), "status 503") {
		t.Fatalf("code = %d out = %q", code, out.String())
	}
	if calls[0].path != "/api/v1/export/destinations/exp_1/test" || calls[0].method != http.MethodPost {
		t.Fatalf("call = %+v", calls[0])
	}
}

func TestRun_ExportDestinationsDeleteAndToggle(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(r *http.Request) (int, any) {
		if r.Method == http.MethodDelete {
			return 204, nil
		}
		return 200, apiclient.ExportDestinationResource{ID: "exp_1", Name: "loki"}
	})
	runCLIExpectOK(t, []string{"export", "destinations", "delete", "exp_1", "--api-url", srv.URL})
	runCLIExpectOK(t, []string{"export", "destinations", "disable", "exp_1", "--api-url", srv.URL})
	if calls[0].method != http.MethodDelete || calls[0].path != "/api/v1/export/destinations/exp_1" {
		t.Fatalf("delete call = %+v", calls[0])
	}
	if calls[1].method != http.MethodPatch || !strings.Contains(calls[1].body, `"enabled":false`) {
		t.Fatalf("disable call = %+v", calls[1])
	}
}

func TestRun_ExportAuditPassesRange(t *testing.T) {
	var calls []exportCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, exportCall{r.Method, r.URL.RequestURI(), ""})
		_, _ = w.Write([]byte("{\"id\":\"aud_1\"}\n"))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"export", "audit", "--from", "2026-10-01T00:00:00Z", "--format", "csv", "--api-url", srv.URL})
	if !strings.Contains(stdout, "aud_1") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(calls[0].path, "from=2026-10-01T00%3A00%3A00Z") || !strings.Contains(calls[0].path, "format=csv") {
		t.Fatalf("path = %q", calls[0].path)
	}
}

func TestRun_ExportDestinationsCreateVendorReadsKeyFromEnv(t *testing.T) {
	var got apiclient.CreateExportDestinationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.ExportDestinationResource{ID: "exp_1", Name: got.Name, Kind: got.Kind, HasAPIKey: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	env := envMap()
	code := run("levelrail-cli-test", []string{
		"export", "destinations", "create", "--name", "nr", "--kind", "logs_newrelic",
		"--api-key-env", "NR_KEY", "--region", "eu", "--api-url", srv.URL,
	}, &stdout, &stderr, func(k string) (string, bool) {
		if k == "NR_KEY" {
			return "license-key", true
		}
		return env(k)
	})
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if got.APIKey != "license-key" || got.Options.Region != "eu" || got.Kind != "logs_newrelic" {
		t.Errorf("request = %+v", got)
	}
	if strings.Contains(stdout.String(), "license-key") {
		t.Errorf("the key was echoed: %q", stdout.String())
	}
}
