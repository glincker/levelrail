package main

import (
	"os"
	"path/filepath"
	"testing"
)

// envMap builds a lookupEnv func (the same shape os.LookupEnv has) from
// a plain map, so precedence tests don't touch real process environment
// variables.
func envMap(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestResolveToken_Precedence(t *testing.T) {
	// No credentials file involved here: HOME is left alone, and no
	// prog collides with a real config dir, so readCredentialsFile
	// simply fails to find one and every case falls through to env or
	// the flag, whichever the test is isolating.
	prog := "levelrail-cli-test-no-such-config-dir"

	tests := []struct {
		name      string
		flagToken string
		env       map[string]string
		want      string
	}{
		{name: "flag wins over env", flagToken: "flag-token", env: map[string]string{envAPIToken: "env-token"}, want: "flag-token"},
		{name: "env used when flag empty", flagToken: "", env: map[string]string{envAPIToken: "env-token"}, want: "env-token"},
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
	prog := "levelrail-cli-test-no-such-config-dir"

	tests := []struct {
		name    string
		flagURL string
		env     map[string]string
		want    string
	}{
		{name: "flag wins over env", flagURL: "http://flag:1", env: map[string]string{envAPIURL: "http://env:2"}, want: "http://flag:1"},
		{name: "env used when flag empty", flagURL: "", env: map[string]string{envAPIURL: "http://env:2"}, want: "http://env:2"},
		{name: "default when nothing set", flagURL: "", env: nil, want: defaultAPIURL},
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

func TestReadCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog"

	dir := filepath.Join(home, ".config", prog)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "# comment\n" + envAPIURL + " = http://file:3 \n" + envAPIToken + "=file-token\n\nnotakeyvalueline\n"
	if err := os.WriteFile(filepath.Join(dir, credentialsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	creds, err := readCredentialsFile(prog)
	if err != nil {
		t.Fatalf("readCredentialsFile() error = %v", err)
	}
	if creds.apiURL != "http://file:3" {
		t.Errorf("apiURL = %q, want %q", creds.apiURL, "http://file:3")
	}
	if creds.token != "file-token" {
		t.Errorf("token = %q, want %q", creds.token, "file-token")
	}
}

func TestReadCredentialsFile_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := readCredentialsFile("no-such-prog"); err == nil {
		t.Fatalf("readCredentialsFile() error = nil, want a not-exist error for a missing file")
	}
}

func TestResolveToken_FallsBackToCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-token-fallback"

	dir := filepath.Join(home, ".config", prog)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := envAPIToken + "=file-token\n"
	if err := os.WriteFile(filepath.Join(dir, credentialsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := resolveToken("", envMap(nil), prog)
	if got != "file-token" {
		t.Errorf("resolveToken() = %q, want %q", got, "file-token")
	}
}
