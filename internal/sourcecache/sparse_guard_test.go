//go:build !windows

package sourcecache

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/kit/netguard"
)

// A fake git on PATH answers the version probe, and records every other run
// (argv and env) so a refused host is proven never to reach it.
func fakeGitOnPath(t *testing.T, version string) (marker, dump string) {
	t.Helper()
	bin, dir := t.TempDir(), t.TempDir()
	marker, dump = filepath.Join(dir, "spawned"), filepath.Join(dir, "dump")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo '" + version + "'; exit 0; fi\n" +
		"echo x >> " + marker + "\n{ echo \"ARGV $*\"; env; echo END; } >> " + dump + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil { //nolint:gosec // test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker, dump
}

func stubLookup(t *testing.T, addrs ...string) {
	t.Helper()
	old := sparseLookup
	t.Cleanup(func() { sparseLookup = old })
	sparseLookup = func(context.Context, string) ([]netip.Addr, error) {
		out := make([]netip.Addr, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}
}

func TestSparseRefusesInternalHostsBeforeGit(t *testing.T) {
	marker, _ := fakeGitOnPath(t, "git version 2.43.0")
	t.Setenv(EnvGitAllowPrivate, "")
	t.Setenv(netguard.AllowPrivateEnv, "")
	stubLookup(t, "10.1.2.3")
	c := New(Config{Dir: t.TempDir()}, nil)
	for _, u := range []string{"https://127.0.0.1/r.git", "https://10.2.3.4:8443/r.git", "https://[::1]/r.git", "https://internal.example/r.git", "http://example.com/r.git"} {
		if _, err := c.SparseCheckout(context.Background(), u, "main", "p", "", 1<<20, time.Second); !errors.Is(err, ErrSparseUnavailable) {
			t.Fatalf("%s: want ErrSparseUnavailable, got %v", u, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("git was started for a refused host")
	}
}

func TestSparsePinCoversEveryAddress(t *testing.T) {
	t.Setenv(EnvGitAllowPrivate, "")
	t.Setenv(netguard.AllowPrivateEnv, "")
	stubLookup(t, "93.184.216.34", "192.168.1.5")
	if _, code, err := sparsePin(context.Background(), "https://example.com/r.git"); err == nil || code != sparseAddrBlocked {
		t.Fatalf("mixed answer must be refused, got %q %v", code, err)
	}
	stubLookup(t, "93.184.216.34")
	pin, _, err := sparsePin(context.Background(), "https://example.com/r.git")
	if err != nil || pin != "example.com:443:93.184.216.34" {
		t.Fatalf("pin = %q, %v", pin, err)
	}
	t.Setenv(EnvGitAllowPrivate, "true")
	if _, _, err := sparsePin(context.Background(), "https://127.0.0.1/r.git"); err != nil {
		t.Fatalf("allow-private should pass: %v", err)
	}
}

func TestSparseHostKeyNormalization(t *testing.T) {
	t.Setenv(EnvGitAllowPrivate, "")
	t.Setenv(netguard.AllowPrivateEnv, "")
	stubLookup(t, "93.184.216.34")
	pin, _, err := sparsePin(context.Background(), "https://Example.COM./r.git")
	if err != nil || pin != "example.com:443:93.184.216.34" {
		t.Fatalf("pin = %q, %v", pin, err)
	}
	for _, u := range []string{"https://exa_mple.com/r.git", "https://b%C3%BCcher.example/r.git", "https://./r.git"} {
		if _, code, err := sparsePin(context.Background(), u); err == nil || code != sparseBadArg {
			t.Fatalf("%s: want bad argument, got %q %v", u, code, err)
		}
	}
	stubLookup(t, "2606:4700::1111")
	pin, _, err = sparsePin(context.Background(), "https://example.com:8443/r.git")
	if err != nil || pin != "example.com:8443:[2606:4700::1111]" {
		t.Fatalf("v6 pin = %q, %v", pin, err)
	}
}

func TestParseGitVersion(t *testing.T) {
	cases := map[string]bool{
		"git version 2.43.0":                 true,
		"git version 2.37.0 (Apple Git-136)": true,
		"git version 2.34.1":                 false,
		"git version 3.0.0":                  true,
		"git version 2.50.1.windows.1":       true,
		"git version 2.36.9":                 false,
		"":                                   false,
		"something else":                     false,
		"git version dev":                    false,
		"git version 2.x":                    false,
		"git version 2.37-rc1":               true,
		"git version 1.99.0":                 false,
	}
	for in, want := range cases {
		maj, minor, ok := parseGitVersion(in)
		got := ok && (maj > sparseMinGitMajor || (maj == sparseMinGitMajor && minor >= sparseMinGitMinor))
		if got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestSparseNeedsRecentGit(t *testing.T) {
	_, dump := fakeGitOnPath(t, "git version 2.34.1")
	t.Setenv(netguard.AllowPrivateEnv, "")
	stubLookup(t, "93.184.216.34")
	c := New(Config{Dir: t.TempDir()}, nil)
	_, err := c.SparseCheckout(context.Background(), "https://example.com/r.git", "main", "p", "", 1<<20, time.Second)
	if !errors.Is(err, ErrSparseUnavailable) || !strings.Contains(err.Error(), sparseOldGit) {
		t.Fatalf("want old git refusal, got %v", err)
	}
	if _, err := os.Stat(dump); err == nil {
		t.Fatal("clone started with an old git")
	}
}

func TestSparseGitRunShape(t *testing.T) {
	_, dump := fakeGitOnPath(t, "git version 2.43.0")
	t.Setenv(EnvGitAllowPrivate, "")
	t.Setenv(netguard.AllowPrivateEnv, "")
	t.Setenv("APP_TEST_SERVER_SECRET", "must-not-reach-git")
	t.Setenv("APP_ENCRYPTION_KEY", "must-not-reach-git")
	c := New(Config{Dir: t.TempDir()}, nil)
	for _, ip := range []string{"93.184.216.34", "2606:4700::1111"} {
		stubLookup(t, ip)
		if _, err := c.SparseCheckout(context.Background(), "https://example.com/r.git", "main", "p", "tok", 1<<20, time.Second); !errors.Is(err, ErrSparseUnavailable) {
			t.Fatalf("want fallback signal, got %v", err)
		}
	}
	raw, err := os.ReadFile(dump) //nolint:gosec // path from t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	runs := strings.Split(strings.TrimSpace(string(raw)), "END")
	for _, r := range runs {
		if strings.TrimSpace(r) == "" {
			continue
		}
		if !strings.Contains(r, "-c http.followRedirects=false") {
			t.Errorf("run without redirects off:\n%s", r)
		}
		if !strings.Contains(r, "http.curloptResolve=example.com:443:") {
			t.Errorf("run without the resolve pin:\n%s", r)
		}
		if strings.Contains(r, "must-not-reach-git") {
			t.Error("server env reached git")
		}
	}
	if !strings.Contains(string(raw), "curloptResolve=example.com:443:93.184.216.34") || !strings.Contains(string(raw), "curloptResolve=example.com:443:[2606:4700::1111]") {
		t.Errorf("v4 and v6 pin forms missing:\n%s", raw)
	}
}

func TestSweepSparseTemp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	old, fresh, other := filepath.Join(tmp, sparseTempPrefix+"old"), filepath.Join(tmp, sparseTempPrefix+"new"), filepath.Join(tmp, "keep")
	for _, d := range []string{old, fresh, other} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	for _, d := range []string{old, other} {
		if err := os.Chtimes(d, past, past); err != nil {
			t.Fatal(err)
		}
	}
	SweepSparseTemp(time.Minute)
	if _, err := os.Stat(old); err == nil {
		t.Error("stale dir kept")
	}
	for _, d := range []string{fresh, other} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s removed", d)
		}
	}
}
