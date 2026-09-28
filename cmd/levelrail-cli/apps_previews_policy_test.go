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

func TestRun_AppsPreviewsApprove_RequiresYesThenPostsConfirm(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(b)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"deploying"}`))
	}))
	defer srv.Close()

	var outBuf, errBuf bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "previews", "approve", "web", "42", "--api-url", srv.URL}, &outBuf, &errBuf, envMap())
	if got == exitOK || gotPath != "" {
		t.Fatalf("approve without --yes: exit = %d, request path = %q, want a refusal before any request", got, gotPath)
	}
	if !strings.Contains(errBuf.String(), "fork") || !strings.Contains(errBuf.String(), "secrets") {
		t.Errorf("stderr = %q, want the security implication in plain words", errBuf.String())
	}

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "approve", "web", "42", "--yes", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/previews/42/approve" || !strings.Contains(gotBody, `"confirm":true`) {
		t.Errorf("request = %s %s %s", gotMethod, gotPath, gotBody)
	}
	if !strings.Contains(stdout, "approved") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsPreviewsLimits_ShowsAndSetsPolicy(t *testing.T) {
	var gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotBody = r.Method, string(b)
		_ = json.NewEncoder(w).Encode(apiclient.PreviewPolicyResource{OnLimit: "reject", TTLHours: 12, EffectiveTTLHours: 12, MaxPerApp: 5, LiveCount: 2, MaxTotal: 0, LiveTotal: 3})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "limits", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || !strings.Contains(stdout, "2 of 5") || !strings.Contains(stdout, "unlimited") || !strings.Contains(stdout, "wait for approval") {
		t.Errorf("show: method = %s, stdout = %q", gotMethod, stdout)
	}

	runCLIExpectOK(t, []string{"apps", "previews", "limits", "web", "--on-limit", "reject", "--allow-forks", "--ttl-hours", "12", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || !strings.Contains(gotBody, `"on_limit":"reject"`) || !strings.Contains(gotBody, `"allow_fork_previews":true`) || !strings.Contains(gotBody, `"ttl_hours":12`) {
		t.Errorf("set: method = %s, body = %s", gotMethod, gotBody)
	}
}

func TestRun_AppsPreviewsList_AcrossAllApps(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, apiclient.PreviewsOverview{
		Previews: []apiclient.PreviewEnvironmentResource{{AppName: "web", PRNumber: 7, PreviewAppID: "web-pr-7", Status: "awaiting_approval", ExpiresAt: "2026-10-01T00:00:00Z"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/previews" || !strings.Contains(stdout, "awaiting_approval") || !strings.Contains(stdout, "2026-10-01") {
		t.Errorf("path = %q, stdout = %q", gotPath, stdout)
	}
}
