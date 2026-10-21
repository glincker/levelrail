package treestat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkCountsRegularFilesOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"x": "12345", "a/y": "123", "a/b/z": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "x"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := Walk(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 3 || got.Bytes != 8 {
		t.Fatalf("got %+v, want 3 files 8 bytes", got)
	}
}

func TestWalkMissingRoot(t *testing.T) {
	if _, err := Walk(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for a missing root")
	}
}

func TestWalkHonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Walk(ctx, t.TempDir()); err == nil {
		t.Fatal("want error for a canceled context")
	}
}
