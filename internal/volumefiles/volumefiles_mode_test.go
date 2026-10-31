//nolint:gosec // test fixtures set and read file modes directly
package volumefiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func modeOf(t *testing.T, p string) fs.FileMode {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func TestStore_WriteModePrivateKeyIs0600(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.WriteMode("certs/k.key", strings.NewReader("k1"), 0, 0o600); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "certs", "k.key")
	if m := modeOf(t, key); m != 0o600 {
		t.Fatalf("new key mode = %o, want 600", m)
	}
	// An atomic replace through plain Write keeps the private mode.
	if _, err := s.Write("certs/k.key", strings.NewReader("k2"), 0); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, key); m != 0o600 {
		t.Fatalf("replaced key mode = %o, want 600", m)
	}
	if b, _ := os.ReadFile(key); string(b) != "k2" {
		t.Fatalf("content = %q", b)
	}

	// A private write over a world readable file narrows it.
	if _, err := s.Write("certs/old.key", strings.NewReader("a"), 0); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, filepath.Join(dir, "certs", "old.key")); m != 0o644 {
		t.Fatalf("default mode = %o, want 644", m)
	}
	if _, err := s.WriteMode("certs/old.key", strings.NewReader("b"), 0, 0o600); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, filepath.Join(dir, "certs", "old.key")); m != 0o600 {
		t.Fatalf("narrowed mode = %o, want 600", m)
	}

	// Never wider than the file it replaces.
	if err := os.Chmod(key, 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteMode("certs/k.key", strings.NewReader("k3"), 0, 0o600); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, key); m != 0o400 {
		t.Fatalf("mode = %o, want 400 (never wider)", m)
	}
}
