package apiclient

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
	// prog collides with a real config dir, so ReadCredentialsFile
	// simply fails to find one and every case falls through to env or
	// the flag, whichever the test is isolating.
	prog := "levelrail-test-no-such-config-dir"

	tests := []struct {
		name      string
		flagToken string
		env       map[string]string
		want      string
	}{
		{name: "flag wins over env", flagToken: "flag-token", env: map[string]string{EnvAPIToken: "env-token"}, want: "flag-token"},
		{name: "env used when flag empty", flagToken: "", env: map[string]string{EnvAPIToken: "env-token"}, want: "env-token"},
		{name: "empty when nothing set", flagToken: "", env: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveToken(tt.flagToken, envMap(tt.env), prog)
			if got != tt.want {
				t.Errorf("ResolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAPIURL_Precedence(t *testing.T) {
	prog := "levelrail-test-no-such-config-dir"

	tests := []struct {
		name    string
		flagURL string
		env     map[string]string
		want    string
	}{
		{name: "flag wins over env", flagURL: "http://flag:1", env: map[string]string{EnvAPIURL: "http://env:2"}, want: "http://flag:1"},
		{name: "env used when flag empty", flagURL: "", env: map[string]string{EnvAPIURL: "http://env:2"}, want: "http://env:2"},
		{name: "default when nothing set", flagURL: "", env: nil, want: DefaultAPIURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAPIURL(tt.flagURL, envMap(tt.env), prog)
			if got != tt.want {
				t.Errorf("ResolveAPIURL() = %q, want %q", got, tt.want)
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
	content := "# comment\n" + EnvAPIURL + " = http://file:3 \n" + EnvAPIToken + "=file-token\n\nnotakeyvalueline\n"
	if err := os.WriteFile(filepath.Join(dir, CredentialsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	creds, err := ReadCredentialsFile(prog)
	if err != nil {
		t.Fatalf("ReadCredentialsFile() error = %v", err)
	}
	if creds.APIURL != "http://file:3" {
		t.Errorf("APIURL = %q, want %q", creds.APIURL, "http://file:3")
	}
	if creds.Token != "file-token" {
		t.Errorf("Token = %q, want %q", creds.Token, "file-token")
	}
}

func TestReadCredentialsFile_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := ReadCredentialsFile("no-such-prog"); err == nil {
		t.Fatalf("ReadCredentialsFile() error = nil, want a not-exist error for a missing file")
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
	content := EnvAPIToken + "=file-token\n"
	if err := os.WriteFile(filepath.Join(dir, CredentialsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := ResolveToken("", envMap(nil), prog)
	if got != "file-token" {
		t.Errorf("ResolveToken() = %q, want %q", got, "file-token")
	}
}

func TestWriteCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-write"

	if err := WriteCredentialsFile(prog, Credentials{APIURL: "http://write:4", Token: "written-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile() error = %v", err)
	}

	creds, err := ReadCredentialsFile(prog)
	if err != nil {
		t.Fatalf("ReadCredentialsFile() error = %v", err)
	}
	if creds.APIURL != "http://write:4" || creds.Token != "written-token" {
		t.Errorf("round-tripped credentials = %+v, want APIURL=http://write:4 Token=written-token", creds)
	}
}
