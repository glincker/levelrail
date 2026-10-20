package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FlagsCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody featureFlagRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(featureFlagResource{
			ID: "ff_1", Key: gotBody.Key, Name: gotBody.Name, ServiceName: "web",
			Enabled: gotBody.Enabled, RolloutPercentage: gotBody.RolloutPercentage,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "create", "web", "--key", "new-checkout", "--name", "New checkout", "--rollout", "50", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/flags" {
		t.Errorf("path = %q, want /api/v1/apps/web/flags", gotPath)
	}
	if gotBody.Key != "new-checkout" || gotBody.Name != "New checkout" || !gotBody.Enabled || gotBody.RolloutPercentage != 50 {
		t.Errorf("request body = %+v, unexpected", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "ff_1"`) {
		t.Errorf("stdout = %q, want the created flag as JSON", stdout.String())
	}
}

func TestRun_FlagsCreate_Disabled(t *testing.T) {
	var gotBody featureFlagRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(featureFlagResource{ID: "ff_1", Key: gotBody.Key, Name: gotBody.Name, ServiceName: "web", Enabled: gotBody.Enabled, RolloutPercentage: gotBody.RolloutPercentage})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "create", "web", "--key", "k", "--name", "n", "--disabled", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false with --disabled")
	}
	if !strings.Contains(stdout.String(), `feature flag "k" (ff_1) created for app "web"`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_FlagsCreate_MissingKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "create", "web", "--name", "n"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--key is required") {
		t.Errorf("stderr = %q, want a missing --key error", stderr.String())
	}
}

func TestRun_FlagsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "create", "web", "--key", "k"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_FlagsCreate_NoAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "create", "--key", "k", "--name", "n"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_FlagsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/flags" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/flags", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]featureFlagResource{
			{ID: "ff_1", Key: "new-checkout", Name: "New checkout", ServiceName: "web", Enabled: true, RolloutPercentage: 100},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "new-checkout") || !strings.Contains(stdout.String(), "100%") {
		t.Errorf("stdout = %q, want the flag listed with its key and rollout", stdout.String())
	}
}

func TestRun_FlagsList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]featureFlagResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no feature flags") {
		t.Errorf("stdout = %q, want the empty-list message", stdout.String())
	}
}

func TestRun_FlagsGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/flags/ff_1" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/flags/ff_1", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(featureFlagResource{
			ID: "ff_1", Key: "new-checkout", Name: "New checkout", ServiceName: "web", Enabled: true, RolloutPercentage: 30,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"flags", "get", "web", "ff_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "key:         new-checkout") {
		t.Errorf("stdout = %q, want the flag key line", stdout)
	}
	if !strings.Contains(stdout, "rollout:     30%") {
		t.Errorf("stdout = %q, want the rollout percentage line", stdout)
	}
}

func TestRun_FlagsGet_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"feature flag not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"flags", "get", "web", "ff_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "feature flag not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_FlagsSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody featureFlagRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(featureFlagResource{
			ID: "ff_1", Key: "new-checkout", Name: gotBody.Name, ServiceName: "web", Enabled: gotBody.Enabled, RolloutPercentage: gotBody.RolloutPercentage,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "set", "web", "ff_1", "--name", "New checkout v2", "--rollout", "10", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/flags/ff_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/flags/ff_1", gotPath)
	}
	if gotBody.Name != "New checkout v2" || gotBody.RolloutPercentage != 10 || !gotBody.Enabled {
		t.Errorf("request body = %+v, unexpected", gotBody)
	}
	if !strings.Contains(stdout.String(), `feature flag "new-checkout" updated`) {
		t.Errorf("stdout = %q, want an update confirmation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "takes effect immediately") {
		t.Errorf("stdout = %q, want the confirmation to make clear this is a live change, no restart needed", stdout.String())
	}
}

func TestRun_FlagsSet_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "set", "web", "ff_1"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a missing --name error", stderr.String())
	}
}

func TestRun_FlagsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "delete", "web", "ff_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/flags/ff_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/flags/ff_1", gotPath)
	}
	if !strings.Contains(stdout.String(), `feature flag "ff_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout.String())
	}
}

func TestRun_Flags_MissingFlagID(t *testing.T) {
	for _, verb := range []string{"get", "set", "delete"} {
		t.Run(verb, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"flags", verb, "web"}, &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "requires") {
				t.Errorf("stderr = %q, want a usage error", stderr.String())
			}
		})
	}
}

func TestRun_Flags_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "flags create") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_Flags_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown flags subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_Flags_NoSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"flags"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
