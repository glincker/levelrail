package build

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateInlinePath(t *testing.T) {
	for _, p := range []string{"../x", "a/../../x", "/etc/passwd", "a//b", "./a", "a/./b", "a\\b", "Dockerfile", "", ".", "..", "c:x"} {
		if err := ValidateInlinePath(p); !errors.Is(err, ErrUnsafeInlinePath) {
			t.Errorf("ValidateInlinePath(%q) = %v, want unsafe", p, err)
		}
	}
	for _, p := range []string{"a.txt", "conf/nginx.conf", ".dockerignore", "src/main.go"} {
		if err := ValidateInlinePath(p); err != nil {
			t.Errorf("ValidateInlinePath(%q) = %v, want ok", p, err)
		}
	}
}

func TestValidateInlineFileSet(t *testing.T) {
	bad := [][]string{
		{"a.txt", "A.TXT"},
		{"café", "café"},
		{"dockerfile"},
		{"a", "a/b"},
		{"A", "a/b/c"},
		{"x", "x"},
	}
	for _, set := range bad {
		if err := ValidateInlineFileSet(set); !errors.Is(err, ErrUnsafeInlinePath) {
			t.Errorf("ValidateInlineFileSet(%q) = %v, want unsafe", set, err)
		}
	}
	if err := ValidateInlineFileSet([]string{"a/b", "a/c", "ab"}); err != nil {
		t.Errorf("distinct set refused: %v", err)
	}
}

func TestMaterializeInlineContext_WritesPrivateFiles(t *testing.T) {
	dir, cleanup, err := MaterializeInlineContext("FROM scratch\n", []InlineFile{{Path: "conf/app.conf", Content: "x=1"}})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "conf", "app.conf")) //nolint:gosec // test temp dir
	if err != nil || string(b) != "x=1" {
		t.Fatalf("app.conf = %q, %v", b, err)
	}
	for name, want := range map[string]os.FileMode{"Dockerfile": 0o600, "conf": 0o700 | os.ModeDir, "conf/app.conf": 0o600} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || fi.Mode() != want {
			t.Errorf("%s mode = %v, %v, want %v", name, fi.Mode(), err, want)
		}
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup left %s behind: %v", dir, err)
	}
}

func TestWriteInlineContext_RefusesTraversalAndSymlinks(t *testing.T) {
	if _, _, err := MaterializeInlineContext("FROM scratch", []InlineFile{{Path: "../escape", Content: "x"}}); !errors.Is(err, ErrUnsafeInlinePath) {
		t.Fatalf("traversal err = %v", err)
	}
	outside := t.TempDir()
	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteInlineContext(dir, "FROM scratch", []InlineFile{{Path: "link/owned", Content: "x"}}); !errors.Is(err, ErrUnsafeInlinePath) {
		t.Fatalf("symlink dir err = %v, want unsafe", err)
	}
	fileDir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "f"), filepath.Join(fileDir, "file")); err != nil {
		t.Fatal(err)
	}
	if err := WriteInlineContext(fileDir, "FROM scratch", []InlineFile{{Path: "file", Content: "x"}}); !errors.Is(err, ErrUnsafeInlinePath) {
		t.Fatalf("symlink file err = %v, want unsafe", err)
	}
	if err := WriteInlineContext(t.TempDir(), "FROM scratch", []InlineFile{{Path: "a", Content: "1"}, {Path: "a", Content: "2"}}); !errors.Is(err, ErrUnsafeInlinePath) {
		t.Fatalf("duplicate name err = %v, want unsafe", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the context: %v", entries)
	}
}
