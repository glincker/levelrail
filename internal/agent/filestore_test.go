package agent

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStoreConfinesPaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	fs, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"../x", "a/../../x", "/etc/passwd", "", "link/x", "a\\..\\..\\x"} {
		if err := fs.Put(rel, strings.NewReader("data"), 0); !errors.Is(err, ErrFilePathEscapes) {
			t.Errorf("Put(%q) error = %v, want ErrFilePathEscapes", rel, err)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("a write escaped the storage root: %v", entries)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	fs, err := NewFileStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("backups/db/1.dump", strings.NewReader("hello"), 0); err != nil {
		t.Fatal(err)
	}
	rc, err := fs.Open("backups/db/1.dump")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello" {
		t.Fatalf("read back %q", got)
	}

	list, err := fs.List("backups/")
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := io.ReadAll(list)
	if want := "5\tbackups/db/1.dump\n"; string(lines) != want {
		t.Fatalf("List = %q, want %q", lines, want)
	}

	if err := fs.Delete("backups/db/1.dump"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete("backups/db/1.dump"); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}
}

func TestFileStorePutEnforcesLimitAndLeavesNoPartial(t *testing.T) {
	root := t.TempDir()
	fs, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("big", bytes.NewReader(make([]byte, 100)), 10); err == nil {
		t.Fatal("Put over the limit succeeded")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("partial object left behind: %v", entries)
	}
}

func TestFileStoreDisabledWithoutRoot(t *testing.T) {
	if _, err := NewFileStore(""); !errors.Is(err, ErrFileStoreDisabled) {
		t.Fatalf("error = %v, want ErrFileStoreDisabled", err)
	}
}
