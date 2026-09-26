package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type scServer struct {
	*httptest.Server
	calls []string
	body  string
}

func newSCServer(t *testing.T) *scServer {
	t.Helper()
	s := &scServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls = append(s.calls, r.Method+" "+r.URL.RequestURI())
		b, _ := io.ReadAll(r.Body)
		s.body = string(b)
		switch {
		case r.URL.Path == "/api/v1/apps/web/deploy-attempts":
			_, _ = w.Write([]byte(`[{"id":"da_new"},{"id":"da_sbom","sbom_packages":3,"vuln_counts":{"critical":1,"high":0,"medium":0,"low":0,"unknown":0}}]`))
		case r.URL.Path == "/api/v1/apps/web/deployments/da_sbom/sbom" && r.URL.Query().Get("download") == "true":
			_, _ = w.Write([]byte(`{"spdxVersion":"SPDX-2.3"}`))
		case r.URL.Path == "/api/v1/apps/web/deployments/da_sbom/sbom":
			_, _ = w.Write([]byte(`{"deployment_id":"da_sbom","format":"spdx","package_count":3,"types":[{"type":"apk","count":3}],"licenses":[{"license":"MIT","count":2}],"unlicensed":1,"top_packages":[],"provenance":true,"available":true,"bytes":10,"generated_at":"2026-09-26T12:00:00Z"}`))
		case r.URL.Path == "/api/v1/apps/web/deployments/da_sbom/vulnerabilities", r.URL.Path == "/api/v1/apps/web/deployments/da_sbom/scan":
			_, _ = w.Write([]byte(`{"deployment_id":"da_sbom","scan":{"status":"ok","scanner":"trivy","counts":{"critical":1,"high":0,"medium":0,"low":0,"unknown":0},"fixable":1,"top":[{"id":"CVE-1","package":"musl","version":"1.2","fixed_version":"1.3","severity":"critical"}]},"gate":{"action":"block","reason":"1 critical"}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/web/supply-chain"):
			_, _ = w.Write([]byte(`{"app":"web","scan_enabled":true,"scan_gate":"block_on_critical","server_enabled":true,"build_attest":false,"scanner":"trivy","scanner_image":"x","override_armed":true,"override_reason":"outage"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func runSC(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	return code, stdout.String(), stderr.String()
}

func TestRun_AppsSBOM_ShowsLatestDeployWithAnSBOM(t *testing.T) {
	srv := newSCServer(t)
	code, out, errOut := runSC(t, "apps", "sbom", "web", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	for _, want := range []string{"da_sbom", "spdx, 3 packages", "apk 3", "MIT 2", "1 without a license", "provenance"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRun_AppsSBOM_Download(t *testing.T) {
	srv := newSCServer(t)
	code, out, _ := runSC(t, "apps", "sbom", "web", "da_sbom", "--download", "--api-url", srv.URL)
	if code != exitOK || out != `{"spdxVersion":"SPDX-2.3"}` {
		t.Fatalf("exit %d out %q", code, out)
	}
	file := filepath.Join(t.TempDir(), "sbom.json")
	if code, _, errOut := runSC(t, "apps", "sbom", "web", "da_sbom", "--file", file, "--api-url", srv.URL); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	//nolint:gosec // file is a temp path the test just wrote
	if data, err := os.ReadFile(file); err != nil || string(data) != `{"spdxVersion":"SPDX-2.3"}` {
		t.Errorf("file = %q, %v", data, err)
	}
}

func TestRun_AppsSBOM_NoDeployWithData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[{"id":"da_new"}]`)) }))
	defer srv.Close()
	if code, _, errOut := runSC(t, "apps", "sbom", "web", "--api-url", srv.URL); code == exitOK || !strings.Contains(errOut, "APP_BUILD_ATTEST") {
		t.Errorf("exit %d stderr %q", code, errOut)
	}
}

func TestRun_AppsScan_Verbs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCall   string
		wantBody   string
		wantOutput string
	}{
		{"enable", []string{"enable", "web"}, "PUT /api/v1/apps/web/supply-chain", `{"scan_enabled":true}`, "scanning:   on"},
		{"disable also clears the gate", []string{"disable", "web"}, "PUT /api/v1/apps/web/supply-chain", `{"scan_enabled":false,"scan_gate":"off"}`, "scanning:"},
		{"gate", []string{"gate", "web", "block_on_critical"}, "PUT /api/v1/apps/web/supply-chain", `{"scan_gate":"block_on_critical"}`, "gate block_on_critical"},
		{"override", []string{"override", "web", "--reason", "outage"}, "POST /api/v1/apps/web/supply-chain/override", `{"reason":"outage"}`, "override:   armed"},
		{"run", []string{"run", "web"}, "POST /api/v1/apps/web/deployments/da_sbom/scan", "", "1 critical"},
		{"status", []string{"status", "web"}, "GET /api/v1/apps/web/deployments/da_sbom/vulnerabilities", "", "gate:       block"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSCServer(t)
			code, out, errOut := runSC(t, append(append([]string{"apps", "scan"}, tc.args...), "--api-url", srv.URL)...)
			if code != exitOK {
				t.Fatalf("exit %d: %s %s", code, out, errOut)
			}
			if tc.wantBody != "" {
				var want, got map[string]any
				_ = json.Unmarshal([]byte(tc.wantBody), &want)
				if err := json.Unmarshal([]byte(srv.body), &got); err != nil || len(got) != len(want) {
					t.Errorf("body = %q, want %q", srv.body, tc.wantBody)
				}
			}
			found := false
			for _, c := range srv.calls {
				found = found || c == tc.wantCall
			}
			if !found {
				t.Errorf("calls = %v, want %q", srv.calls, tc.wantCall)
			}
			if !strings.Contains(out, tc.wantOutput) {
				t.Errorf("output missing %q:\n%s", tc.wantOutput, out)
			}
		})
	}
}

func TestRun_AppsScan_Validation(t *testing.T) {
	srv := newSCServer(t)
	for _, args := range [][]string{
		{"apps", "scan"},
		{"apps", "scan", "bogus", "web"},
		{"apps", "scan", "enable"},
		{"apps", "scan", "gate", "web"},
		{"apps", "scan", "override", "web"},
	} {
		if code, _, _ := runSC(t, append(args, "--api-url", srv.URL)...); code == exitOK {
			t.Errorf("%v must fail", args)
		}
	}
	if len(srv.calls) != 0 {
		t.Errorf("validation failures must not reach the server: %v", srv.calls)
	}
}
