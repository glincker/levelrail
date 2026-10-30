package dockerssh

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name     string
	typeflag byte
	body     string
	link     string
	mode     int64
}

func buildTar(t *testing.T, entries []entry) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Linkname: e.link, Mode: e.mode, Size: int64(len(e.body))}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if e.typeflag != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	return bytes.NewReader(buf.Bytes())
}

func TestExtractArchiveHappyPath(t *testing.T) {
	dst := t.TempDir()
	st, err := ExtractArchive(buildTar(t, []entry{
		{name: "data/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "data/sub/", typeflag: tar.TypeDir, mode: 0o555},
		{name: "data/sub/a.txt", typeflag: tar.TypeReg, body: "hello"},
		{name: "data/link", typeflag: tar.TypeSymlink, link: "sub/a.txt"},
	}), dst, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 2 || st.Bytes != 5 {
		t.Fatalf("stats = %+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "sub", "a.txt")); string(b) != "hello" { //nolint:gosec // path inside the test temp dir
		t.Fatalf("content = %q", b)
	}
	if l, err := os.Readlink(filepath.Join(dst, "link")); err != nil || l != "sub/a.txt" {
		t.Fatalf("link = %q, %v", l, err)
	}
}

func TestExtractArchiveRefusesPathTraversal(t *testing.T) {
	for _, name := range []string{"data/../../evil", "data/../evil", "../evil", "data/a/../../../evil"} {
		parent := t.TempDir()
		dst := filepath.Join(parent, "vol")
		if err := os.Mkdir(dst, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := ExtractArchive(buildTar(t, []entry{{name: name, typeflag: tar.TypeReg, body: "x"}}), dst, Limits{})
		if err == nil {
			t.Errorf("%q was extracted", name)
		}
		if _, serr := os.Stat(filepath.Join(parent, "evil")); serr == nil {
			t.Errorf("%q wrote outside the target", name)
		}
	}
}

func TestExtractArchiveSymlinkEscapes(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "vol")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := ExtractArchive(buildTar(t, []entry{
		{name: "data/abs", typeflag: tar.TypeSymlink, link: outside},
		{name: "data/rel", typeflag: tar.TypeSymlink, link: "../outside"},
		{name: "data/deep", typeflag: tar.TypeSymlink, link: "a/../../outside"},
		{name: "data/hard", typeflag: tar.TypeLink, link: "data/abs"},
		{name: "data/dev", typeflag: tar.TypeChar},
	}), dst, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Skipped != 5 {
		t.Fatalf("skipped = %d, want 5 (%v)", st.Skipped, st.Warnings)
	}
	for _, n := range []string{"abs", "rel", "deep", "hard", "dev"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err == nil {
			t.Errorf("%s was created", n)
		}
	}
}

func TestExtractArchiveNeverWritesThroughASymlink(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "vol")
	if err := os.MkdirAll(filepath.Join(dst, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "pre")); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractArchive(buildTar(t, []entry{
		{name: "data/pre/pwn", typeflag: tar.TypeReg, body: "x"},
	}), dst, Limits{})
	if err == nil {
		t.Fatal("wrote below a symlinked directory")
	}
	if _, serr := os.Stat(filepath.Join(outside, "pwn")); serr == nil {
		t.Fatal("file written through the symlink")
	}

	_, err = ExtractArchive(buildTar(t, []entry{
		{name: "data/inner", typeflag: tar.TypeSymlink, link: "real"},
		{name: "data/inner/pwn", typeflag: tar.TypeReg, body: "x"},
	}), dst, Limits{})
	if err == nil {
		t.Fatal("wrote below an in-archive symlinked directory")
	}
	if _, serr := os.Stat(filepath.Join(dst, "real", "pwn")); serr == nil {
		t.Fatal("file written through the in-archive symlink")
	}
}

func TestExtractArchiveFileOverSymlinkReplacesTheLink(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "vol")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(parent, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dst, "f")); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractArchive(buildTar(t, []entry{{name: "data/f", typeflag: tar.TypeReg, body: "new"}}), dst, Limits{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" { //nolint:gosec // path inside the test temp dir
		t.Fatalf("symlink target was overwritten: %q", b)
	}
}

func TestExtractArchiveLimits(t *testing.T) {
	big := strings.Repeat("x", 100)
	_, err := ExtractArchive(buildTar(t, []entry{
		{name: "data/a", typeflag: tar.TypeReg, body: big},
		{name: "data/b", typeflag: tar.TypeReg, body: big},
	}), t.TempDir(), Limits{MaxBytes: 150})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("byte cap: %v", err)
	}
	_, err = ExtractArchive(buildTar(t, []entry{
		{name: "data/a", typeflag: tar.TypeReg, body: "1"},
		{name: "data/b", typeflag: tar.TypeReg, body: "1"},
		{name: "data/c", typeflag: tar.TypeReg, body: "1"},
	}), t.TempDir(), Limits{MaxFiles: 2})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("file cap: %v", err)
	}
}

func TestExtractArchiveRequiresADirectoryTarget(t *testing.T) {
	if _, err := ExtractArchive(buildTar(t, nil), filepath.Join(t.TempDir(), "missing"), Limits{}); err == nil {
		t.Fatal("extracted into a missing directory")
	}
}

func TestExtractArchiveRefusesLinkChainsThatLeaveTheVolume(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "vol")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := ExtractArchive(buildTar(t, []entry{
		{name: "data/d", typeflag: tar.TypeDir},
		{name: "data/d/l", typeflag: tar.TypeSymlink, link: "."},
		{name: "data/e", typeflag: tar.TypeSymlink, link: "d/l/../.."},
	}), dst, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, lerr := os.Lstat(filepath.Join(dst, "e")); lerr == nil {
		t.Fatalf("link chain that resolves outside the volume was created (%v)", st.Warnings)
	}
	if _, lerr := os.Lstat(filepath.Join(dst, "d", "l")); lerr != nil {
		t.Fatalf("a harmless in-volume link should still be created: %v", lerr)
	}
}

func TestExtractArchiveDirectoryEntryNeverFollowsASymlink(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "vol")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "e")); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractArchive(buildTar(t, []entry{{name: "data/e", typeflag: tar.TypeDir, mode: 0o777}}), dst, Limits{}); err == nil {
		t.Fatal("a directory entry over a symlink must be refused")
	}
	fi, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o500 {
		t.Fatalf("directory outside the volume was chmodded to %v", fi.Mode().Perm())
	}
}
