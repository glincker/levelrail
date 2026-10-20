package network

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateKey_GeneratesAndPersistsOnFirstCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.key")

	key, err := LoadOrGenerateKey(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey: %v", err)
	}
	if key.String() == "" {
		t.Fatal("LoadOrGenerateKey() returned an empty key")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat persisted key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("persisted key file mode = %o, want 0600", perm)
	}
}

func TestLoadOrGenerateKey_ReloadsSameKeyOnSecondCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.key")

	first, err := LoadOrGenerateKey(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey (first): %v", err)
	}
	second, err := LoadOrGenerateKey(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey (second): %v", err)
	}
	if first != second {
		t.Fatalf("LoadOrGenerateKey() = %v on reload, want the persisted key %v", second, first)
	}
}

func TestLoadOrGenerateKey_ParsesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.key")

	want, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	if err := os.WriteFile(path, []byte(want.String()), 0o600); err != nil {
		t.Fatalf("seed key file: %v", err)
	}

	got, err := LoadOrGenerateKey(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey: %v", err)
	}
	if got != want {
		t.Fatalf("LoadOrGenerateKey() = %v, want the seeded key %v", got, want)
	}
}

func TestLoadOrGenerateKey_MalformedFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.key")
	if err := os.WriteFile(path, []byte("not a valid key"), 0o600); err != nil {
		t.Fatalf("seed malformed key file: %v", err)
	}

	if _, err := LoadOrGenerateKey(path); err == nil {
		t.Fatal("LoadOrGenerateKey() error = nil, want a parse error for a malformed key file")
	}
}

func TestPersistKey_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "mesh.key")

	key, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	if err := PersistKey(path, key); err != nil {
		t.Fatalf("PersistKey: %v", err)
	}

	raw, err := os.ReadFile(path) //nolint:gosec // t.TempDir()-scoped test path, not user input
	if err != nil {
		t.Fatalf("read persisted key: %v", err)
	}
	if string(raw) != key.String() {
		t.Fatalf("persisted key file content = %q, want %q", raw, key.String())
	}
}
