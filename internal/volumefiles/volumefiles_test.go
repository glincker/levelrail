//nolint:gosec // test fixtures need readable files and real symlinks
package volumefiles

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newVolume(t *testing.T) (*Store, string, string) {
	t.Helper()
	base := t.TempDir()
	vol := filepath.Join(base, "vol")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{vol, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("host secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(vol)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, vol, outside
}

func TestStore_RoundTrip(t *testing.T) {
	s, _, _ := newVolume(t)
	if _, err := s.Write("conf/app.ini", strings.NewReader("a=1"), 1024); err != nil {
		t.Fatalf("write: %v", err)
	}
	rc, size, err := s.Read("/conf/app.ini")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, _ := io.ReadAll(rc)
	if string(got) != "a=1" || size != 3 {
		t.Fatalf("got %q size %d", got, size)
	}
	entries, truncated, err := s.List("conf", 10)
	if err != nil || truncated || len(entries) != 1 || entries[0].Path != "conf/app.ini" || entries[0].Type != "file" {
		t.Fatalf("list: %+v truncated=%v err=%v", entries, truncated, err)
	}
	if err := s.Remove("conf/app.ini"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, _, err := s.Read("conf/app.ini"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read after remove = %v, want ErrNotFound", err)
	}
}

func TestStore_RejectsTraversal(t *testing.T) {
	s, _, _ := newVolume(t)
	for _, p := range []string{"../outside/secret.txt", "a/../../outside/secret.txt", "..", "a/../..", "x\x00y"} {
		if _, _, err := s.Read(p); !errors.Is(err, ErrPathEscapes) {
			t.Errorf("Read(%q) = %v, want ErrPathEscapes", p, err)
		}
		if _, err := s.Write(p, strings.NewReader("x"), 10); !errors.Is(err, ErrPathEscapes) {
			t.Errorf("Write(%q) = %v, want ErrPathEscapes", p, err)
		}
		if err := s.Remove(p); !errors.Is(err, ErrPathEscapes) {
			t.Errorf("Remove(%q) = %v, want ErrPathEscapes", p, err)
		}
		if _, _, err := s.List(p, 10); !errors.Is(err, ErrPathEscapes) {
			t.Errorf("List(%q) = %v, want ErrPathEscapes", p, err)
		}
	}
}

func TestStore_AbsolutePathStaysInsideVolume(t *testing.T) {
	s, vol, outside := newVolume(t)
	if _, err := s.Write(filepath.Join(outside, "secret.txt"), strings.NewReader("pwn"), 10); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(outside, "secret.txt"))
	if string(b) != "host secret" {
		t.Fatalf("outside file modified: %q", b)
	}
	if _, err := os.Stat(filepath.Join(vol, outside, "secret.txt")); err != nil {
		t.Fatalf("absolute path should land under the volume root: %v", err)
	}
}

func TestStore_SymlinkEscapesRefused(t *testing.T) {
	s, vol, outside := newVolume(t)
	if err := os.Symlink(outside, filepath.Join(vol, "dirlink")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(vol, "filelink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside/secret.txt", filepath.Join(vol, "rellink")); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"dirlink/secret.txt", "filelink", "rellink"} {
		if _, _, err := s.Read(p); err == nil {
			t.Errorf("Read(%q) followed a symlink out of the volume", p)
		}
	}
	if _, err := s.Write("dirlink/new.txt", strings.NewReader("pwn"), 10); err == nil {
		t.Error("Write through a directory symlink escaped the volume")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Error("file created outside the volume")
	}
	if _, _, err := s.List("dirlink", 10); err == nil {
		t.Error("List followed a directory symlink out of the volume")
	}
	if err := s.Remove("dirlink/secret.txt"); err == nil {
		t.Error("Remove through a directory symlink escaped the volume")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Errorf("outside file gone: %v", err)
	}
}

func TestStore_WriteOverSymlinkReplacesLinkNotTarget(t *testing.T) {
	s, vol, outside := newVolume(t)
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(vol, "filelink")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := s.Write("filelink", strings.NewReader("new"), 10); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(outside, "secret.txt"))
	if string(b) != "host secret" {
		t.Fatalf("symlink target modified: %q", b)
	}
}

func TestStore_ListSymlinkNotFollowed(t *testing.T) {
	s, vol, outside := newVolume(t)
	if err := os.Symlink(outside, filepath.Join(vol, "dirlink")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	entries, _, err := s.List(".", 10)
	if err != nil || len(entries) != 1 || entries[0].Type != "symlink" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}

func TestStore_UploadCapAndNoPartialFile(t *testing.T) {
	s, vol, _ := newVolume(t)
	_, err := s.Write("big.bin", bytes.NewReader(make([]byte, 100)), 10)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	left, _ := os.ReadDir(vol)
	if len(left) != 0 {
		t.Fatalf("partial files left behind: %v", left)
	}
}

func TestStore_RefusesDirectoriesAndRoot(t *testing.T) {
	s, vol, _ := newVolume(t)
	if err := os.Mkdir(filepath.Join(vol, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("d"); !errors.Is(err, ErrIsDirectory) {
		t.Errorf("Remove(dir) = %v", err)
	}
	if err := s.Remove("/"); !errors.Is(err, ErrIsDirectory) {
		t.Errorf("Remove(root) = %v", err)
	}
	if _, err := s.Write("d", strings.NewReader("x"), 10); !errors.Is(err, ErrIsDirectory) {
		t.Errorf("Write(dir) = %v", err)
	}
	if _, _, err := s.Read("d"); !errors.Is(err, ErrIsDirectory) {
		t.Errorf("Read(dir) = %v", err)
	}
}

func TestStore_WriteKeepsMode(t *testing.T) {
	s, vol, _ := newVolume(t)
	p := filepath.Join(vol, "run.sh")
	if err := os.WriteFile(p, []byte("old"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("run.sh", strings.NewReader("new"), 10); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o750 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
}

func TestStore_ListLimitTruncates(t *testing.T) {
	s, vol, _ := newVolume(t)
	for _, n := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(vol, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, truncated, err := s.List(".", 2)
	if err != nil || !truncated || len(entries) != 2 {
		t.Fatalf("entries=%d truncated=%v err=%v", len(entries), truncated, err)
	}
	var buf bytes.Buffer
	if err := EncodeEntries(&buf, entries, truncated); err != nil {
		t.Fatal(err)
	}
	back, tr, err := DecodeEntries(&buf)
	if err != nil || !tr || len(back) != 2 {
		t.Fatalf("decode: %d %v %v", len(back), tr, err)
	}
}

func TestEnvLimits(t *testing.T) {
	t.Setenv(envMaxUploadMB, "2")
	t.Setenv(envListLimit, "5")
	if MaxUploadBytes() != 2<<20 || ListLimit() != 5 {
		t.Fatalf("env not honored: %d %d", MaxUploadBytes(), ListLimit())
	}
	if EffectiveMax(1024) != 1024 || EffectiveMax(0) != 2<<20 || EffectiveMax(1<<30) != 2<<20 {
		t.Fatal("EffectiveMax")
	}
	if EffectiveLimit(3) != 3 || EffectiveLimit(0) != 5 || EffectiveLimit(99) != 5 {
		t.Fatal("EffectiveLimit")
	}
}
