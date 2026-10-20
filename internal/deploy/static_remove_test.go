package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveStaticTree(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "site", "sha"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"", ".", "..", "../x", "a/b", "/etc", "site/sha"} {
		if err := RemoveStaticTree(root, bad); !errors.Is(err, ErrUnsafeStaticName) {
			t.Errorf("name %q: err = %v, want ErrUnsafeStaticName", bad, err)
		}
	}
	if err := RemoveStaticTree("", "site"); err == nil {
		t.Error("empty root accepted")
	}

	if err := RemoveStaticTree(root, "site"); err != nil {
		t.Fatalf("remove site: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "site")); !os.IsNotExist(err) {
		t.Errorf("site tree still present: %v", err)
	}
	if err := RemoveStaticTree(root, "site"); err != nil {
		t.Errorf("second remove = %v, want nil", err)
	}

	if err := RemoveStaticTree(root, "link"); err != nil {
		t.Fatalf("remove link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Errorf("symlink target contents deleted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "link")); !os.IsNotExist(err) {
		t.Errorf("link still present: %v", err)
	}
}
