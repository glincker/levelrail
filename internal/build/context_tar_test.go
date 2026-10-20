package build

import (
	"archive/tar"
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTarContext_RoundTrip(t *testing.T) {
	src := t.TempDir()
	writeFixture(t, filepath.Join(src, "Dockerfile"), "FROM scratch\n", 0o644)
	writeFixture(t, filepath.Join(src, "app", "main.go"), "package main\n", 0o644)
	writeFixture(t, filepath.Join(src, "scripts", "build.sh"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("Dockerfile", filepath.Join(src, "Dockerfile.link")); err != nil {
		t.Fatalf("creating fixture symlink: %v", err)
	}

	var buf bytes.Buffer
	if err := TarContext(t.Context(), src, &buf); err != nil {
		t.Fatalf("TarContext() error = %v", err)
	}

	dst := t.TempDir()
	if err := UntarContext(t.Context(), &buf, dst); err != nil {
		t.Fatalf("UntarContext() error = %v", err)
	}

	for _, rel := range []string{"Dockerfile", filepath.Join("app", "main.go"), filepath.Join("scripts", "build.sh")} {
		want, err := os.ReadFile(filepath.Join(src, rel)) //nolint:gosec // test fixture path
		if err != nil {
			t.Fatalf("reading fixture %q: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(dst, rel)) //nolint:gosec // test fixture path
		if err != nil {
			t.Fatalf("extracted context is missing %q: %v", rel, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%q content = %q, want %q", rel, got, want)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "scripts", "build.sh"))
		if err != nil {
			t.Fatalf("stat extracted script: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("extracted script mode = %v, want 0755: an executable build step must stay executable", info.Mode().Perm())
		}
	}

	link, err := os.Readlink(filepath.Join(dst, "Dockerfile.link"))
	if err != nil {
		t.Fatalf("extracted context is missing the symlink: %v", err)
	}
	if link != "Dockerfile" {
		t.Errorf("symlink target = %q, want %q", link, "Dockerfile")
	}
}

// TestTarContext_SkipsIrregularEntries covers the one class of entry a
// build context can contain but must not carry: a socket left behind by
// a dev server in the checkout would otherwise fail the whole archive
// rather than being skipped the way Docker's own context upload skips it.
func TestTarContext_SkipsIrregularEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix socket fixture is not meaningful on windows")
	}
	// Not t.TempDir(): a unix socket path has a hard length limit well
	// below what a temp dir named after this test fits in.
	src, err := os.MkdirTemp("", "lr-tar-*")
	if err != nil {
		t.Fatalf("creating fixture dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(src) })

	writeFixture(t, filepath.Join(src, "Dockerfile"), "FROM scratch\n", 0o644)
	if err := os.MkdirAll(filepath.Join(src, "empty"), 0o750); err != nil {
		t.Fatalf("creating fixture dir: %v", err)
	}
	listener, err := net.Listen("unix", filepath.Join(src, "s.sock"))
	if err != nil {
		t.Skipf("skipping: could not create a unix socket fixture: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var buf bytes.Buffer
	if err := TarContext(t.Context(), src, &buf); err != nil {
		t.Fatalf("TarContext() error = %v", err)
	}

	names := tarEntryNames(t, buf.Bytes())
	if !names["Dockerfile"] || !names["empty/"] {
		t.Errorf("entries = %v, want both the file and the empty directory", names)
	}
	if names["s.sock"] {
		t.Error("entries include the unix socket, want it skipped")
	}
}

func TestUntarContext_RejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name  string
		entry tar.Header
	}{
		{name: "parent traversal", entry: tar.Header{Name: "../escaped", Typeflag: tar.TypeReg, Mode: 0o644}},
		{name: "nested parent traversal", entry: tar.Header{Name: "app/../../escaped", Typeflag: tar.TypeReg, Mode: 0o644}},
		{name: "absolute path", entry: tar.Header{Name: "/etc/escaped", Typeflag: tar.TypeReg, Mode: 0o644}},
		{name: "empty name", entry: tar.Header{Name: "", Typeflag: tar.TypeReg, Mode: 0o644}},
		{name: "escaping symlink", entry: tar.Header{Name: "link", Linkname: "../outside", Typeflag: tar.TypeSymlink, Mode: 0o777}},
		{name: "nested escaping symlink", entry: tar.Header{Name: "a/b/link", Linkname: "../../../outside", Typeflag: tar.TypeSymlink, Mode: 0o777}},
		{name: "absolute symlink", entry: tar.Header{Name: "link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink, Mode: 0o777}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			header := tt.entry
			header.Size = 0
			if err := tw.WriteHeader(&header); err != nil {
				t.Fatalf("writing crafted header: %v", err)
			}
			if err := tw.Close(); err != nil {
				t.Fatalf("closing crafted tar: %v", err)
			}

			err := UntarContext(t.Context(), &buf, t.TempDir())
			if !errors.Is(err, ErrUnsafeContextPath) {
				t.Fatalf("UntarContext() err = %v, want %v", err, ErrUnsafeContextPath)
			}
		})
	}
}

// TestUntarContext_RejectsSymlinkChainEscape covers a zip-slip variant
// the lexical checks alone would pass: a symlink to "." makes a second
// link's "../x" target resolve one level above the root on disk.
func TestUntarContext_RejectsSymlinkChainEscape(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "ctx")
	if err := os.Mkdir(dst, 0o750); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range []tar.Header{
		{Name: "p", Linkname: ".", Typeflag: tar.TypeSymlink, Mode: 0o777},
		{Name: "p/q", Linkname: "../escaped", Typeflag: tar.TypeSymlink, Mode: 0o777},
		{Name: "p/q/owned", Typeflag: tar.TypeReg, Mode: 0o644},
	} {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("writing crafted header: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing crafted tar: %v", err)
	}

	if err := UntarContext(t.Context(), &buf, dst); !errors.Is(err, ErrUnsafeContextPath) {
		t.Fatalf("UntarContext() err = %v, want %v", err, ErrUnsafeContextPath)
	}
	if _, err := os.Lstat(filepath.Join(parent, "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("something was written outside the context root: %v", err)
	}
}

// TestUntarContext_PreexistingEscapingSymlinkCannotRedirectWrite covers
// the os.Root backstop: a link already in the destination is not trusted.
func TestUntarContext_PreexistingEscapingSymlinkCannotRedirectWrite(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "ctx")
	outside := filepath.Join(parent, "outside")
	for _, d := range []string{dst, outside} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dst, "dir")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("owned")
	if err := tw.WriteHeader(&tar.Header{Name: "dir/owned", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := UntarContext(t.Context(), &buf, dst); err == nil {
		t.Fatal("UntarContext() error = nil, want the write through an escaping symlink refused")
	}
	if _, err := os.Lstat(filepath.Join(outside, "owned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file was written outside the context root: %v", err)
	}
}

func TestUntarContext_RejectsTruncatedStream(t *testing.T) {
	src := t.TempDir()
	writeFixture(t, filepath.Join(src, "Dockerfile"), strings.Repeat("FROM scratch\n", 200), 0o644)

	var buf bytes.Buffer
	if err := TarContext(t.Context(), src, &buf); err != nil {
		t.Fatalf("TarContext() error = %v", err)
	}

	// Half a context is not a context: a build must fail rather than run
	// against a partially transferred tree.
	truncated := bytes.NewReader(buf.Bytes()[:buf.Len()/2])
	if err := UntarContext(t.Context(), truncated, t.TempDir()); err == nil {
		t.Fatal("UntarContext() on a truncated stream: error = nil, want a non-nil error")
	}
}

func writeFixture(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating fixture dir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("writing fixture %q: %v", path, err)
	}
}

func tarEntryNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := tr.Next()
		if err != nil {
			return names
		}
		names[header.Name] = true
	}
}
