package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// TestMain enables every experimental feature so the rest of the package
// exercises the gated commands; the gate tests narrow it per case.
func TestMain(m *testing.M) {
	experimental.Set(experimental.All()...)
	os.Exit(m.Run())
}

func TestExperimentalCommandsGated(t *testing.T) {
	tests := []struct {
		args    []string
		feature experimental.Feature
		usage   string
	}{
		{[]string{"models", "list"}, experimental.AIModels, "models list"},
		{[]string{"lb", "show", "web"}, experimental.LoadBalancer, "lb show"},
		{[]string{"apply", "-f", "x.yaml"}, experimental.IaC, "apply -f"},
		{[]string{"diff", "-f", "x.yaml"}, experimental.IaC, ""},
		{[]string{"export"}, experimental.IaC, "export [--project"},
		{[]string{"cloudflare-tunnel", "get"}, experimental.CloudflareTunnel, "cloudflare-tunnel get"},
		{[]string{"settings", "ai-assistant", "get"}, experimental.AIChat, "and the BYOK AI assistant"},
	}
	defer experimental.Set(experimental.All()...)
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			experimental.Set()
			var stdout, stderr bytes.Buffer
			code := run("prog", tc.args, &stdout, &stderr, func(string) (string, bool) { return "", false })
			if code != exitUsage {
				t.Fatalf("code = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), experimental.EnvVar) {
				t.Fatalf("stderr does not name %s: %q", experimental.EnvVar, stderr.String())
			}
			if tc.usage != "" && strings.Contains(rootUsage("prog"), tc.usage) {
				t.Fatalf("help still lists %q while off", tc.usage)
			}
			experimental.Set(tc.feature)
			if tc.usage != "" && !strings.Contains(rootUsage("prog"), tc.usage) {
				t.Fatalf("help hides %q while on", tc.usage)
			}
			if rejectDisabledExperimental("prog", tc.args, &stderr) {
				t.Fatal("rejected while on")
			}
		})
	}
}
