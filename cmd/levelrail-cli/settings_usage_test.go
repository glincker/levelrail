package main

import (
	"strings"
	"testing"
)

func TestSettingsUsage_ListsEverySubtree(t *testing.T) {
	u := settingsUsage("levelrail-cli")
	for _, want := range []string{
		"settings email branding get|set|preview",
		"settings heartbeat show|set|clear",
		"settings ddns show|set|enable|disable|status",
		"settings runtime list|get|set|reset|apply",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("settings usage lacks %q", want)
		}
	}
}

func TestInternalCANote(t *testing.T) {
	if internalCANote("https://a:18703", "letsencrypt") != "" {
		t.Error("no note expected for a public CA")
	}
	if !strings.Contains(internalCANote("https://a:18703", certKindInternal), "https://a:18703") {
		t.Error("internal CA note must name the full URL")
	}
}

func TestUpAsDeployArgs(t *testing.T) {
	got := strings.Join(upAsDeployArgs(upFlags{name: "x", port: 80}, "d", true, true, "5m", "", "", "", true, outputFlags{}), " ")
	for _, want := range []string{"--app x", "--port 80", "--json", "--dry-run", "--yes", "--timeout 5m"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q lack %q", got, want)
		}
	}
	if !strings.HasSuffix(got, " d") {
		t.Errorf("directory must come last: %q", got)
	}
}
