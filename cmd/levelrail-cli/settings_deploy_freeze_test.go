package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SettingsDeployFreezeSetAndShow(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	until := time.Date(2026, 9, 28, 7, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		res := apiclient.DeployFreezeResource{Windows: []apiclient.FreezeWindowResource{{Cron: "0 17 * * 5", Duration: "64h0m0s", Timezone: "Europe/Berlin", Reason: "weekend"}}}
		res.Status.Frozen, res.Status.Until = true, &until
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "deploy-freeze", "set", "--cron", "0 17 * * 5", "--duration", "64h", "--timezone", "Europe/Berlin", "--reason", "weekend", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/deploy-freeze" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"cron":"0 17 * * 5"`) || !strings.Contains(gotBody, `"duration":"64h"`) {
		t.Fatalf("request body = %s", gotBody)
	}
	if !strings.Contains(stdout, "FROZEN until 2026-09-28T07:00:00Z") {
		t.Fatalf("stdout = %q", stdout)
	}

	stdout, _ = runCLIExpectOK(t, []string{"settings", "deploy-freeze", "show", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/deploy-freeze" {
		t.Fatalf("show request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "weekend") {
		t.Fatalf("show stdout = %q", stdout)
	}
}

func TestRun_SettingsDeployFreezeClear(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DeployFreezeResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "deploy-freeze", "clear", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/deploy-freeze" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"windows":[]`) {
		t.Fatalf("clear sent %s", gotBody)
	}
	if !strings.Contains(stdout, "no freeze windows") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_SettingsDeployFreezeSet_MissingFlags(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"settings", "deploy-freeze", "set"})
	if !strings.Contains(stderr, "--cron and --duration are required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRun_SettingsDeployFreezeShow_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)
	stderr := runCLIExpectAPIError(t, []string{"settings", "deploy-freeze", "show", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRun_SettingsDeployFreeze_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"settings", "deploy-freeze", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown settings deploy-freeze subcommand") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRun_SettingsDeployFreeze_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "deploy-freeze", "-h"})
	if !strings.Contains(stdout, "settings deploy-freeze show") {
		t.Errorf("stdout = %q", stdout)
	}
}
