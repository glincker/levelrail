package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func envMap(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestResolveToken_Precedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "levelrail-mcp-test-no-such-config-dir"

	tests := []struct {
		name      string
		flagToken string
		env       map[string]string
		want      string
	}{
		{name: "flag wins over env", flagToken: "flag-token", env: map[string]string{apiclient.EnvAPIToken: "env-token"}, want: "flag-token"},
		{name: "env used when flag empty", flagToken: "", env: map[string]string{apiclient.EnvAPIToken: "env-token"}, want: "env-token"},
		{name: "empty when nothing set", flagToken: "", env: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveToken(tt.flagToken, envMap(tt.env), prog)
			if got != tt.want {
				t.Errorf("resolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAPIURL_Precedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "levelrail-mcp-test-no-such-config-dir"

	tests := []struct {
		name    string
		flagURL string
		env     map[string]string
		want    string
	}{
		{name: "flag wins over env", flagURL: "http://flag:1", env: map[string]string{apiclient.EnvAPIURL: "http://env:2"}, want: "http://flag:1"},
		{name: "env used when flag empty", flagURL: "", env: map[string]string{apiclient.EnvAPIURL: "http://env:2"}, want: "http://env:2"},
		{name: "default when nothing set", flagURL: "", env: nil, want: apiclient.DefaultAPIURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAPIURL(tt.flagURL, envMap(tt.env), prog)
			if got != tt.want {
				t.Errorf("resolveAPIURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveToken_FallsBackToCLICredentials is the "operator who
// already ran levelrail-cli auth login doesn't need to set anything up
// twice" contract: this server's own credentials file is empty/missing,
// but the CLI's has a token.
func TestResolveToken_FallsBackToCLICredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", cliProg)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := apiclient.EnvAPIToken + "=cli-token\n" + apiclient.EnvAPIURL + "=http://cli-configured:9\n"
	if err := os.WriteFile(filepath.Join(dir, apiclient.CredentialsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotToken := resolveToken("", envMap(nil), "levelrail-mcp-no-own-credentials")
	if gotToken != "cli-token" {
		t.Errorf("resolveToken() = %q, want %q (fallback to the CLI's credentials file)", gotToken, "cli-token")
	}
	gotURL := resolveAPIURL("", envMap(nil), "levelrail-mcp-no-own-credentials")
	if gotURL != "http://cli-configured:9" {
		t.Errorf("resolveAPIURL() = %q, want %q (fallback to the CLI's credentials file)", gotURL, "http://cli-configured:9")
	}
}

// TestResolveToken_OwnCredentialsWinOverCLI: this server's own
// credentials file, when present, takes precedence over the CLI's
// fallback (checked first, in resolveToken's loop order).
func TestResolveToken_OwnCredentialsWinOverCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "levelrail-mcp-test-own-config"

	for _, entry := range []struct {
		prog  string
		token string
	}{
		{prog: prog, token: "own-token"},
		{prog: cliProg, token: "cli-token"},
	} {
		dir := filepath.Join(home, ".config", entry.prog)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		content := apiclient.EnvAPIToken + "=" + entry.token + "\n"
		if err := os.WriteFile(filepath.Join(dir, apiclient.CredentialsFileName), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	got := resolveToken("", envMap(nil), prog)
	if got != "own-token" {
		t.Errorf("resolveToken() = %q, want %q (own credentials file wins)", got, "own-token")
	}
}
