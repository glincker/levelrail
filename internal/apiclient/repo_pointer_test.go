package apiclient

import (
	"os"
	"path/filepath"
	"testing"
)

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestResolveProfileSource_PointerPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".levelrail"), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "profile=work\napi_url=https://work.example.com\nAPP_API_TOKEN=must-be-ignored\n"
	if err := os.WriteFile(filepath.Join(root, RepoPointerPath), []byte(body), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	cases := []struct {
		name, flag, env, want, wantSource string
	}{
		{"flag wins", "flagged", "envd", "flagged", SourceFlag},
		{"env beats pointer", "", "envd", "envd", SourceEnv},
		{"pointer beats default, found by walking up", "", "", "work", SourceRepo},
	}
	for _, c := range cases {
		env := map[string]string{}
		if c.env != "" {
			env[EnvProfile] = c.env
		}
		got, src := ResolveProfileSource(c.flag, lookupFrom(env))
		if got != c.want || src != c.wantSource {
			t.Errorf("%s: got %q from %q, want %q from %q", c.name, got, src, c.want, c.wantSource)
		}
	}
	if u := ResolveAPIURL("", lookupFrom(nil), "lr-test-none", "work"); u != "https://work.example.com" {
		t.Errorf("pointer URL for its own profile = %q", u)
	}
	if u := ResolveAPIURL("", lookupFrom(nil), "lr-test-none", "other"); u != DefaultAPIURL {
		t.Errorf("pointer URL must not leak to another profile, got %q", u)
	}
	if u := ResolveAPIURL("https://flag.example.com", lookupFrom(nil), "lr-test-none", "work"); u != "https://flag.example.com" {
		t.Errorf("flag must beat the pointer, got %q", u)
	}
}

func TestFindRepoPointer_NoneAndSecretKeysIgnored(t *testing.T) {
	dir := t.TempDir()
	if _, ok := FindRepoPointer(dir); ok {
		t.Fatal("no pointer expected in an empty tree")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".levelrail"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, RepoPointerPath), []byte("token=abc\n"), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if _, ok := FindRepoPointer(dir); ok {
		t.Fatal("a file with only unknown keys must not count as a pointer")
	}
}

func TestWriteCredentialsFile_KeepsTrustedDeviceAndMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const prog = "lr-test-device"
	if err := WriteCredentialsFile(prog, "p", Credentials{APIURL: "http://x", Token: "t1", TrustedDevice: "dev-1", TokenExpiresAt: "2030-01-01T00:00:00Z"}); err != nil { //nolint:gosec // test fixture values
		t.Fatal(err)
	}
	if err := WriteCredentialsFile(prog, "p", Credentials{APIURL: "http://x", Token: "t2"}); err != nil {
		t.Fatal(err)
	}
	c, err := ReadCredentialsFile(prog, "p")
	if err != nil {
		t.Fatal(err)
	}
	if c.TrustedDevice != "dev-1" || c.Token != "t2" || c.TokenExpiresAt != "" {
		t.Errorf("got %+v, want the device id kept, token replaced, expiry cleared", c)
	}
	st, err := os.Stat(filepath.Join(home, ".config", prog, CredentialsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("credentials mode = %v, want 0600", st.Mode().Perm())
	}
}
