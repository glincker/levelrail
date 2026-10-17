package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SettingsAIAssistant_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, aiAssistantSettingsResource{Configured: true, Active: &apiclient.AIModelRef{Provider: "anthropic", Model: "claude-sonnet-4-5"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "get", "--api-url", srv.URL})

	if gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("path = %s, want /api/v1/settings/ai-assistant", gotPath)
	}
	if !strings.Contains(stdout, "configured: true") || !strings.Contains(stdout, "active:     anthropic:claude-sonnet-4-5") {
		t.Errorf("stdout = %q, want configured/model lines", stdout)
	}
}

func TestRun_SettingsAIAssistant_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"settings", "ai-assistant", "-h"})
	if !strings.Contains(stdout, "settings ai-assistant providers") || strings.Contains(stdout, "ai-assistant set") {
		t.Errorf("stdout = %q, want the provider-based usage text only", stdout)
	}
}

func TestRun_SettingsAIAssistant_RemovedVerbs(t *testing.T) {
	for _, verb := range []string{"set", "clear"} {
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", []string{"settings", "ai-assistant", verb}, &stdout, &stderr, envMap()); got != exitUsage {
			t.Errorf("%s: exit = %d, want %d", verb, got, exitUsage)
		}
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
