package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrateverify"
)

func verifyServer(t *testing.T, rep apiclient.MigrationVerifyReport, got *map[string]any, path *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.Method + " " + r.URL.Path
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func verifyEnv(extra map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == envAPIToken {
			return "control-plane-token", true
		}
		v, ok := extra[k]
		return v, ok
	}
}

func sampleVerifyReport(blocking int) apiclient.MigrationVerifyReport {
	r := migrateverify.Report{Platform: "coolify", Categories: []migrateverify.Totals{{Category: migrateverify.CatEnv, Source: 2, Target: 1, Matched: 1}},
		Differences: []migrateverify.Difference{{Category: migrateverify.CatEnv, Severity: migrateverify.Info, App: "web", Subject: "EXTRA", Detail: "variable exists here but not on the source", Fix: "nothing to do"}},
		Probes:      []migrateverify.DomainProbe{{Domain: "web.example.com", Source: migrateverify.ProbeResult{Status: 200, LatencyMS: 40, TLS: true, TLSValid: true, TLSDaysLeft: 60}, Target: migrateverify.ProbeResult{Error: "refused"}}}}
	if blocking > 0 {
		r.Differences = append(r.Differences, migrateverify.Difference{Category: migrateverify.CatApps, Severity: migrateverify.Blocking, Subject: "shop", Detail: "not found on this instance", Fix: "import it"})
	}
	r.Finish()
	return apiclient.MigrationVerifyReport{Report: r}
}

func TestMigrateVerifyExitCodeAndTable(t *testing.T) {
	tests := []struct {
		name     string
		blocking int
		want     int
	}{
		{"blocking difference exits non-zero", 1, exitCheckFailed},
		{"no blocking difference exits zero", 0, exitOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			var path string
			srv := verifyServer(t, sampleVerifyReport(tc.blocking), &body, &path)
			var out, errOut bytes.Buffer
			got := run("levelrail-cli-test", []string{"migrate", "verify", "coolify", "--url", "https://coolify.example.com", "--token", "src-secret-token",
				"--api-url", srv.URL, "--probe-timeout", "7s", "--probe-concurrency", "3", "--target-ip", "203.0.113.9", "--only", "web"}, &out, &errOut, verifyEnv(nil))
			if got != tc.want {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, tc.want, errOut.String())
			}
			if path != "POST /api/v1/migrations/verify" {
				t.Errorf("request = %s", path)
			}
			if body["platform"] != "coolify" || body["probe_timeout_seconds"] != float64(7) || body["probe_concurrency"] != float64(3) || body["target_ip"] != "203.0.113.9" {
				t.Errorf("request body = %v", body)
			}
			text := out.String()
			for _, want := range []string{"AREA", "SEVERITY", "variable exists here", "nothing to do", "web.example.com", "error: refused"} {
				if !strings.Contains(text, want) {
					t.Errorf("table lacks %q:\n%s", want, text)
				}
			}
			if strings.Contains(text, "src-secret-token") {
				t.Error("the source token was printed")
			}
		})
	}
}

func TestMigrateVerifyJSONOutputAndEnvDefaults(t *testing.T) {
	var body map[string]any
	var path string
	srv := verifyServer(t, sampleVerifyReport(1), &body, &path)
	var out, errOut bytes.Buffer
	env := map[string]string{migrateverify.EnvProbeTimeout: "20s", migrateverify.EnvProbeConcurrency: "9"}
	env[envMigrateSourceToken] = "env-source-token"
	got := run("levelrail-cli-test", []string{"migrate", "verify", "dokploy", "--url", "https://d.example.com", "--api-url", srv.URL, "--output", "json"},
		&out, &errOut, verifyEnv(env))
	if got != exitCheckFailed {
		t.Fatalf("exit = %d, stderr %q", got, errOut.String())
	}
	if body["token"] != "env-source-token" || body["probe_timeout_seconds"] != float64(20) || body["probe_concurrency"] != float64(9) {
		t.Errorf("env defaults not applied: %v", body)
	}
	var rep apiclient.MigrationVerifyReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || rep.Counts.Blocking != 1 {
		t.Fatalf("stdout is not the report JSON: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "env-source-token") {
		t.Error("the source token was printed")
	}
}

func TestMigrateVerifyLatestAndRecheckNeedNoSource(t *testing.T) {
	var body map[string]any
	var path string
	srv := verifyServer(t, sampleVerifyReport(0), &body, &path)
	var out, errOut bytes.Buffer
	if got := run("levelrail-cli-test", []string{"migrate", "verify", "--latest", "--api-url", srv.URL}, &out, &errOut, verifyEnv(nil)); got != exitOK || path != "GET /api/v1/migrations/verify/latest" {
		t.Fatalf("latest: exit %d, request %q, stderr %q", got, path, errOut.String())
	}
	out.Reset()
	if got := run("levelrail-cli-test", []string{"migrate", "verify", "--recheck", "--api-url", srv.URL}, &out, &errOut, verifyEnv(nil)); got != exitOK || body["recheck"] != true {
		t.Fatalf("recheck: exit %d, body %v, stderr %q", got, body, errOut.String())
	}
}

func TestMigrateVerifyRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown source", []string{"migrate", "verify", "heroku", "--url", "https://x", "--token", "t"}},
		{"missing token", []string{"migrate", "verify", "coolify", "--url", "https://x"}},
		{"bad timeout", []string{"migrate", "verify", "coolify", "--url", "https://x", "--token", "t", "--probe-timeout", "soon"}},
		{"negative concurrency", []string{"migrate", "verify", "coolify", "--url", "https://x", "--token", "t", "--probe-concurrency", "-2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run("levelrail-cli-test", append(tc.args, "--api-url", "http://127.0.0.1:1"), &out, &errOut, verifyEnv(nil)); got != exitValidation {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, exitValidation, errOut.String())
			}
		})
	}
}
