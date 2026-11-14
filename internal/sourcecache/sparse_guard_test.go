package sourcecache

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// A fake git on PATH records any spawn, so a refused host is proven never to reach it.
func fakeGitOnPath(t *testing.T) string {
	t.Helper()
	bin, marker := t.TempDir(), filepath.Join(t.TempDir(), "spawned")
	script := "#!/bin/sh\necho x >> " + marker + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil { //nolint:gosec // test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func TestSparseRefusesInternalHostsBeforeGit(t *testing.T) {
	marker := fakeGitOnPath(t)
	t.Setenv(EnvGitAllowPrivate, "")
	old := sparseLookup
	sparseLookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.1.2.3")}, nil
	}
	t.Cleanup(func() { sparseLookup = old })
	c := New(Config{Dir: t.TempDir()}, nil)
	for _, u := range []string{"https://127.0.0.1/r.git", "https://10.2.3.4:8443/r.git", "https://[::1]/r.git", "https://internal.example/r.git", "http://example.com/r.git"} {
		if _, err := c.SparseCheckout(context.Background(), u, "main", "p", "", 1<<20); !errors.Is(err, ErrSparseUnavailable) {
			t.Fatalf("%s: want ErrSparseUnavailable, got %v", u, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("git was started for a refused host")
	}
}

func TestSparsePinCoversEveryAddress(t *testing.T) {
	t.Setenv(EnvGitAllowPrivate, "")
	old := sparseLookup
	t.Cleanup(func() { sparseLookup = old })
	sparseLookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("192.168.1.5")}, nil
	}
	if _, code, err := sparsePin(context.Background(), "https://example.com/r.git"); err == nil || code != sparseAddrBlocked {
		t.Fatalf("mixed answer must be refused, got %q %v", code, err)
	}
	sparseLookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	pin, _, err := sparsePin(context.Background(), "https://example.com/r.git")
	if err != nil || pin != "example.com:443:93.184.216.34" {
		t.Fatalf("pin = %q, %v", pin, err)
	}
	t.Setenv(EnvGitAllowPrivate, "true")
	if _, _, err := sparsePin(context.Background(), "https://127.0.0.1/r.git"); err != nil {
		t.Fatalf("allow-private should pass: %v", err)
	}
}
