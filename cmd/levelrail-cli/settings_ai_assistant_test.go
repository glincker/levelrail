package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsAIAssistant_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, aiAssistantSettingsResource{Configured: true, Provider: "anthropic", Model: "claude-sonnet-4-5"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "get", "--api-url", srv.URL})

	if gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("path = %s, want /api/v1/settings/ai-assistant", gotPath)
	}
	if !strings.Contains(stdout, "configured: true") || !strings.Contains(stdout, "model:      claude-sonnet-4-5") {
		t.Errorf("stdout = %q, want configured/model lines", stdout)
	}
}

func TestRun_SettingsAIAssistant_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody updateAIAssistantSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(aiAssistantSettingsResource{Configured: true, Provider: gotBody.Provider, Model: gotBody.Model})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "set", "--model", "claude-sonnet-4-5", "--api-key", "sk-ant-test", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if gotBody.Provider != "anthropic" || gotBody.Model != "claude-sonnet-4-5" || gotBody.APIKey != "sk-ant-test" {
		t.Errorf("request body = %+v, want provider=anthropic model=claude-sonnet-4-5 api_key=sk-ant-test", gotBody)
	}
	if !strings.Contains(stdout, "configured: true") {
		t.Errorf("stdout = %q, want configured: true", stdout)
	}
}

func TestRun_SettingsAIAssistant_Set_MissingModel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "ai-assistant", "set", "--api-key", "sk-ant-test", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --model") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_SettingsAIAssistant_Set_MissingAPIKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "ai-assistant", "set", "--model", "claude-sonnet-4-5", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --api-key") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_SettingsAIAssistant_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotImplemented, `{"error":"the ai assistant is not configurable on this control plane (no master key set)"}`)

	stderr := runCLIExpectAPIError(t, []string{"settings", "ai-assistant", "set", "--model", "claude-sonnet-4-5", "--api-key", "sk-ant-test", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no master key set") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_SettingsAIAssistant_Clear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(aiAssistantSettingsResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "clear", "--api-url", srv.URL})

	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "configured: false") {
		t.Errorf("stdout = %q, want configured: false", stdout)
	}
}

func TestRun_SettingsAIAssistant_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "-h"})
	if !strings.Contains(stdout, "settings ai-assistant set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_SettingsAIAssistant_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "ai-assistant", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown settings ai-assistant subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
