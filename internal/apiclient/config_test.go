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
			got := ResolveToken(tt.flagToken, envMap(tt.env), prog, DefaultProfile)
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
			got := ResolveAPIURL(tt.flagURL, envMap(tt.env), prog, DefaultProfile)
			if got != tt.want {
				t.Errorf("ResolveAPIURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveProfile_Precedence(t *testing.T) {
	tests := []struct {
		name        string
		flagProfile string
		env         map[string]string
		want        string
	}{
		{name: "flag wins over env", flagProfile: "flag-profile", env: map[string]string{EnvProfile: "env-profile"}, want: "flag-profile"},
		{name: "env used when flag empty", flagProfile: "", env: map[string]string{EnvProfile: "env-profile"}, want: "env-profile"},
		{name: "default when nothing set", flagProfile: "", env: nil, want: DefaultProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProfile(tt.flagProfile, envMap(tt.env))
			if got != tt.want {
				t.Errorf("ResolveProfile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeConfigFile(t *testing.T, home, prog, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", prog)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, CredentialsFileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestReadCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog"

	writeConfigFile(t, home, prog, "# comment\n"+EnvAPIURL+" = http://file:3 \n"+EnvAPIToken+"=file-token\n\nnotakeyvalueline\n")

	creds, err := ReadCredentialsFile(prog, DefaultProfile)
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

// TestReadCredentialsFile_OldFlatFileIsDefaultProfile confirms a
// credentials file written before profile support existed (no
// "[section]" header at all) still loads as DefaultProfile, so an
// operator's existing saved credentials keep working unmodified.
func TestReadCredentialsFile_OldFlatFileIsDefaultProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-old-flat"

	writeConfigFile(t, home, prog, EnvAPIURL+"=http://old:9\n"+EnvAPIToken+"=old-token\n")

	creds, err := ReadCredentialsFile(prog, DefaultProfile)
	if err != nil {
		t.Fatalf("ReadCredentialsFile() error = %v", err)
	}
	if creds.APIURL != "http://old:9" || creds.Token != "old-token" {
		t.Errorf("creds = %+v, want APIURL=http://old:9 Token=old-token", creds)
	}

	if got := ResolveToken("", envMap(nil), prog, DefaultProfile); got != "old-token" {
		t.Errorf("ResolveToken() = %q, want old-token", got)
	}
	if got := ResolveAPIURL("", envMap(nil), prog, DefaultProfile); got != "http://old:9" {
		t.Errorf("ResolveAPIURL() = %q, want http://old:9", got)
	}
}

func TestReadCredentialsFile_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := ReadCredentialsFile("no-such-prog", DefaultProfile); err == nil {
		t.Fatalf("ReadCredentialsFile() error = nil, want a not-exist error for a missing file")
	}
}

// TestReadCredentialsFile_UnknownProfile confirms asking for a profile
// name that isn't in the file returns a zero Credentials and no error,
// the same "nothing configured" shape a missing file's absence gets
// higher up the chain, not a hard failure.
func TestReadCredentialsFile_UnknownProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-unknown-profile"

	writeConfigFile(t, home, prog, "[default]\n"+EnvAPIURL+"=http://default:1\n"+EnvAPIToken+"=default-token\n")

	creds, err := ReadCredentialsFile(prog, "does-not-exist")
	if err != nil {
		t.Fatalf("ReadCredentialsFile() error = %v", err)
	}
	if creds != (Credentials{}) {
		t.Errorf("creds = %+v, want zero value", creds)
	}
}

func TestResolveToken_FallsBackToCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-token-fallback"

	writeConfigFile(t, home, prog, EnvAPIToken+"=file-token\n")

	got := ResolveToken("", envMap(nil), prog, DefaultProfile)
	if got != "file-token" {
		t.Errorf("ResolveToken() = %q, want %q", got, "file-token")
	}
}

// TestMultipleProfiles_RoundTrip confirms two named profiles saved via
// WriteCredentialsFile coexist in the same file, each resolved
// independently by name, and neither clobbers the other.
func TestMultipleProfiles_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-multi-profile"

	if err := WriteCredentialsFile(prog, DefaultProfile, Credentials{APIURL: "http://home:8080", Token: "home-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile(default) error = %v", err)
	}
	if err := WriteCredentialsFile(prog, "work", Credentials{APIURL: "https://work.example.com", Token: "work-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile(work) error = %v", err)
	}

	homeCreds, err := ReadCredentialsFile(prog, DefaultProfile)
	if err != nil {
		t.Fatalf("ReadCredentialsFile(default) error = %v", err)
	}
	if homeCreds.APIURL != "http://home:8080" || homeCreds.Token != "home-token" {
		t.Errorf("default profile = %+v, want APIURL=http://home:8080 Token=home-token", homeCreds)
	}

	workCreds, err := ReadCredentialsFile(prog, "work")
	if err != nil {
		t.Fatalf("ReadCredentialsFile(work) error = %v", err)
	}
	if workCreds.APIURL != "https://work.example.com" || workCreds.Token != "work-token" {
		t.Errorf("work profile = %+v, want APIURL=https://work.example.com Token=work-token", workCreds)
	}

	if got := ResolveToken("", envMap(nil), prog, "work"); got != "work-token" {
		t.Errorf("ResolveToken(work) = %q, want work-token", got)
	}
	if got := ResolveAPIURL("", envMap(nil), prog, "work"); got != "https://work.example.com" {
		t.Errorf("ResolveAPIURL(work) = %q, want https://work.example.com", got)
	}
}

// TestWriteCredentialsFile_OverwriteProfilePreservesOthers confirms
// re-saving one profile (the common "auth login --profile work" a
// second time, rotating a token) leaves every other profile in the file
// untouched.
func TestWriteCredentialsFile_OverwriteProfilePreservesOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-overwrite"

	if err := WriteCredentialsFile(prog, DefaultProfile, Credentials{APIURL: "http://home:8080", Token: "home-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile(default) error = %v", err)
	}
	if err := WriteCredentialsFile(prog, "work", Credentials{APIURL: "https://work.example.com", Token: "work-token-1"}); err != nil {
		t.Fatalf("WriteCredentialsFile(work) error = %v", err)
	}
	if err := WriteCredentialsFile(prog, "work", Credentials{APIURL: "https://work.example.com", Token: "work-token-2"}); err != nil {
		t.Fatalf("WriteCredentialsFile(work) rewrite error = %v", err)
	}

	homeCreds, err := ReadCredentialsFile(prog, DefaultProfile)
	if err != nil {
		t.Fatalf("ReadCredentialsFile(default) error = %v", err)
	}
	if homeCreds.Token != "home-token" {
		t.Errorf("default profile token = %q, want unchanged home-token", homeCreds.Token)
	}

	workCreds, err := ReadCredentialsFile(prog, "work")
	if err != nil {
		t.Fatalf("ReadCredentialsFile(work) error = %v", err)
	}
	if workCreds.Token != "work-token-2" {
		t.Errorf("work profile token = %q, want rotated work-token-2", workCreds.Token)
	}
}

func TestListProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-list-profiles"

	if err := WriteCredentialsFile(prog, DefaultProfile, Credentials{APIURL: "http://home:8080", Token: "home-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile(default) error = %v", err)
	}
	if err := WriteCredentialsFile(prog, "work", Credentials{APIURL: "https://work.example.com", Token: "work-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile(work) error = %v", err)
	}

	profiles, err := ListProfiles(prog)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("ListProfiles() returned %d profiles, want 2: %+v", len(profiles), profiles)
	}
	if profiles[0] != (ProfileSummary{Name: DefaultProfile, APIURL: "http://home:8080"}) {
		t.Errorf("profiles[0] = %+v, want default/http://home:8080", profiles[0])
	}
	if profiles[1] != (ProfileSummary{Name: "work", APIURL: "https://work.example.com"}) {
		t.Errorf("profiles[1] = %+v, want work/https://work.example.com", profiles[1])
	}
}

func TestListProfiles_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	profiles, err := ListProfiles("no-such-prog-list-profiles")
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if profiles != nil {
		t.Errorf("ListProfiles() = %+v, want nil for a missing file", profiles)
	}
}

func TestWriteCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prog := "test-prog-write"

	if err := WriteCredentialsFile(prog, DefaultProfile, Credentials{APIURL: "http://write:4", Token: "written-token"}); err != nil {
		t.Fatalf("WriteCredentialsFile() error = %v", err)
	}

	creds, err := ReadCredentialsFile(prog, DefaultProfile)
	if err != nil {
		t.Fatalf("ReadCredentialsFile() error = %v", err)
	}
	if creds.APIURL != "http://write:4" || creds.Token != "written-token" {
		t.Errorf("round-tripped credentials = %+v, want APIURL=http://write:4 Token=written-token", creds)
	}
}
