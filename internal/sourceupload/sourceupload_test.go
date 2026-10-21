package sourceupload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func paths(p Plan) []string {
	var out []string
	for _, f := range p.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestScanIgnoreRules(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		include []string
		want    []string
	}{
		{"defaults drop deps secrets and git", map[string]string{
			"main.go": "x", "node_modules/a/index.js": "x", ".git/HEAD": "x", ".env": "S=1", ".env.local": "S=1",
			"deploy.pem": "k", "sub/.env": "S=1", "keep.txt": "x",
		}, nil, []string{"keep.txt", "main.go"}},
		{"env templates stay", map[string]string{".env.example": "A=", "a.txt": "x"}, nil, []string{".env.example", "a.txt"}},
		{"gitignore applies", map[string]string{".gitignore": "dist/\n*.log\n", "dist/a.js": "x", "a.log": "x", "src/b.log": "x", "src/b.go": "x"}, nil, []string{".gitignore", "src/b.go"}},
		{"nested gitignore scoped to its dir", map[string]string{"pkg/.gitignore": "gen.go\n", "pkg/gen.go": "x", "gen.go": "x"}, nil, []string{"gen.go", "pkg/.gitignore"}},
		{"gitignore negation cannot re-admit a secret", map[string]string{".gitignore": "!.env\n", ".env": "S=1", "a": "x"}, nil, []string{".gitignore", "a"}},
		{"explicit include re-adds env", map[string]string{".env": "S=1", "a": "x"}, []string{".env"}, []string{".env", "a"}},
		{"include a directory", map[string]string{"node_modules/x/y.js": "x", "a": "x"}, []string{"node_modules/x"}, []string{"a", "node_modules/x/y.js"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tc.files)
			p, err := Scan(context.Background(), root, Options{Include: tc.include})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(paths(p), ","); got != strings.Join(tc.want, ",") {
				t.Fatalf("files = %s, want %s", got, strings.Join(tc.want, ","))
			}
		})
	}
}

func TestScanSkipsSymlinksAndEnforcesCap(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": strings.Repeat("x", 100)})
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unsupported")
	}
	p, err := Scan(context.Background(), root, Options{})
	if err != nil || len(p.Files) != 1 {
		t.Fatalf("scan = %v, %v", paths(p), err)
	}
	if _, err := Scan(context.Background(), root, Options{MaxBytes: 50}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func packTree(t *testing.T, files map[string]string) []byte {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, files)
	p, err := Scan(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := p.WriteArchive(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackExtractRoundTripIsDeterministic(t *testing.T) {
	files := map[string]string{"Dockerfile": "FROM scratch", "src/main.go": "package main", "src/deep/x.txt": "hello"}
	a, b := packTree(t, files), packTree(t, files)
	if !bytes.Equal(a, b) {
		t.Fatal("same tree packed to different bytes")
	}
	dest := t.TempDir()
	st, err := Extract(bytes.NewReader(a), dest, Limits{})
	if err != nil || st.Files != 3 {
		t.Fatalf("extract = %+v, %v", st, err)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name))) //nolint:gosec // test reads back files it extracted into a temp dir
		if err != nil || string(got) != body {
			t.Fatalf("%s = %q, %v", name, got, err)
		}
	}
}

type entry struct {
	name, link string
	typ        byte
	body       string
	size       int64
}

func makeArchive(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		size := e.size
		if size == 0 {
			size = int64(len(e.body))
		}
		if e.typ == tar.TypeSymlink || e.typ == tar.TypeLink || e.typ == tar.TypeDir {
			size = 0
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.link, Size: size, Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			if _, err := tw.Write([]byte(strings.Repeat("a", int(size)))); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExtractRejectsUnsafeArchives(t *testing.T) {
	tests := []struct {
		name    string
		entries []entry
		lim     Limits
		target  error
	}{
		{"parent traversal", []entry{{name: "../evil", typ: tar.TypeReg, body: "x"}}, Limits{}, ErrUnsafeArchive},
		{"nested traversal", []entry{{name: "a/../../evil", typ: tar.TypeReg, body: "x"}}, Limits{}, ErrUnsafeArchive},
		{"absolute path", []entry{{name: "/etc/cron.d/x", typ: tar.TypeReg, body: "x"}}, Limits{}, ErrUnsafeArchive},
		{"symlink escaping root", []entry{{name: "l", typ: tar.TypeSymlink, link: "/etc"}}, Limits{}, ErrUnsafeArchive},
		{"symlink then write through it", []entry{{name: "l", typ: tar.TypeSymlink, link: "../.."}, {name: "l/pwn", typ: tar.TypeReg, body: "x"}}, Limits{}, ErrUnsafeArchive},
		{"hard link", []entry{{name: "h", typ: tar.TypeLink, link: "../x"}}, Limits{}, ErrUnsafeArchive},
		{"device node", []entry{{name: "d", typ: tar.TypeChar}}, Limits{}, ErrUnsafeArchive},
		{"fifo", []entry{{name: "f", typ: tar.TypeFifo}}, Limits{}, ErrUnsafeArchive},
		{"backslash name", []entry{{name: `..\evil`, typ: tar.TypeReg, body: "x"}}, Limits{}, ErrUnsafeArchive},
		{"too many files", []entry{{name: "a", typ: tar.TypeReg, body: "x"}, {name: "b", typ: tar.TypeReg, body: "x"}, {name: "c", typ: tar.TypeReg, body: "x"}}, Limits{MaxFiles: 2}, ErrTooLarge},
		{"decompression bomb", []entry{{name: "big", typ: tar.TypeReg, size: 4 << 20}}, Limits{MaxBytes: 1 << 20}, ErrTooLarge},
		{"single file over cap", []entry{{name: "big", typ: tar.TypeReg, size: 2 << 20}}, Limits{MaxFileBytes: 1 << 20}, ErrTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			arc := makeArchive(t, tc.entries)
			parent := t.TempDir()
			dest := filepath.Join(parent, "out")
			if err := os.Mkdir(dest, 0o750); err != nil {
				t.Fatal(err)
			}
			if _, err := Extract(bytes.NewReader(arc), dest, tc.lim); !errors.Is(err, tc.target) {
				t.Fatalf("Extract err = %v, want %v", err, tc.target)
			}
			if _, err := Inspect(bytes.NewReader(arc), tc.lim); !errors.Is(err, tc.target) {
				t.Fatalf("Inspect err = %v, want %v", err, tc.target)
			}
			if _, err := os.Stat(filepath.Join(parent, "evil")); err == nil {
				t.Fatal("file escaped the destination")
			}
		})
	}
}

func TestExtractRejectsNonGzip(t *testing.T) {
	if _, err := Inspect(strings.NewReader("not an archive"), Limits{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStorePutDedupeListCleanup(t *testing.T) {
	s := NewStore(t.TempDir(), Limits{})
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	arc := packTree(t, map[string]string{"Dockerfile": "FROM scratch", "a.txt": "a"})

	m, deduped, err := s.Put("web", bytes.NewReader(arc), 0)
	if err != nil || deduped || m.Files != 2 || !ValidHash(m.Hash) {
		t.Fatalf("Put = %+v deduped=%v err=%v", m, deduped, err)
	}
	m2, deduped, err := s.Put("web", bytes.NewReader(arc), 0)
	if err != nil || !deduped || m2.Hash != m.Hash {
		t.Fatalf("second Put = %+v deduped=%v err=%v", m2, deduped, err)
	}
	if list, _ := s.List("web"); len(list) != 1 {
		t.Fatalf("List = %d, want 1", len(list))
	}
	if n, err := s.Usage(); err != nil || n != m.Size {
		t.Fatalf("Usage = %d, %v, want %d", n, err, m.Size)
	}
	dest := t.TempDir()
	if st, err := s.Extract("web", m.Hash, dest); err != nil || st.Files != 2 {
		t.Fatalf("Extract = %+v, %v", st, err)
	}

	if n, _ := s.Cleanup(time.Hour); n != 0 {
		t.Fatalf("fresh upload removed: %d", n)
	}
	now = now.Add(2 * time.Hour)
	if n, err := s.Cleanup(time.Hour); err != nil || n != 1 {
		t.Fatalf("Cleanup = %d, %v, want 1", n, err)
	}
	if _, err := s.Get("web", m.Hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after cleanup = %v, want ErrNotFound", err)
	}
}

func TestStoreRejects(t *testing.T) {
	s := NewStore(t.TempDir(), Limits{})
	arc := packTree(t, map[string]string{"a": strings.Repeat("x", 1000)})
	tests := []struct {
		name string
		app  string
		body []byte
		max  int64
		want error
	}{
		{"over size cap", "web", arc, 10, ErrTooLarge},
		{"bad app name", "../web", arc, 0, ErrInvalidName},
		{"unsafe archive", "web", makeArchive(t, []entry{{name: "../x", typ: tar.TypeReg, body: "x"}}), 0, ErrUnsafeArchive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := s.Put(tc.app, bytes.NewReader(tc.body), tc.max); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := s.Get("web", "../../etc/passwd"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Get traversal hash = %v", err)
	}
	if list, _ := s.List("web"); len(list) != 0 {
		t.Fatal("rejected upload was stored")
	}
}
