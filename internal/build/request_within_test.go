package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSolveOptRootContainment(t *testing.T) {
	outside := t.TempDir()
	src := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(src, "leak")); err != nil {
		t.Fatal(err)
	}
	req := Request{ContextDir: filepath.Join(src, "leak"), Tag: "x:y", RootDir: src}
	if _, err := newSolveOpt(req, CacheConfig{}, nil); err == nil {
		t.Fatal("context resolving outside the root should be refused")
	}
}
