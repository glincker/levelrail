package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type tarEntry struct {
	name   string
	typ    byte
	body   string
	target string
	mode   int64
}

// dockerTar builds the plain tar the archive API returns.
func dockerTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: mode, Linkname: e.target, ModTime: time.Unix(1700000000, 0)}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if typ == tar.TypeDir {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// packFor runs src through a packer and returns the packed tar entries.
func packFor(t *testing.T, decl string, src io.Reader, lim packLimits) ([]*tar.Header, error) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := newPacker(tw, lim).add(decl, src, 0, "")
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var hs []*tar.Header
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return hs, nil
		}
		if err != nil {
			t.Fatal(err)
		}
		hs = append(hs, h)
	}
}

func TestCleanArtifactPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"dist", "dist", false},
		{"./out/app", "out/app", false},
		{"a/../b", "b", false},
		{".", "", true},
		{"", "", true},
		{"../etc/passwd", "", true},
		{"a/../../etc", "", true},
		{"/etc/passwd", "", true},
		{"~/x", "", true},
		{"dist/*.js", "", true},
		{"bad\nname", "", true},
	}
	for _, tc := range tests {
		got, err := cleanArtifactPath(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("cleanArtifactPath(%q) = %q, %v", tc.in, got, err)
		}
	}
}

func TestPackerEntryMatrix(t *testing.T) {
	t.Parallel()
	many := make([]tarEntry, 0, 12)
	many = append(many, tarEntry{name: "dist/", typ: tar.TypeDir})
	for i := 0; i < 10; i++ {
		many = append(many, tarEntry{name: "dist/f" + string(rune('a'+i)), body: "x"})
	}
	long := strings.Repeat("d/", 100) + "file"
	tests := []struct {
		name    string
		entries []tarEntry
		lim     packLimits
		wantErr string
	}{
		{"plain files", []tarEntry{{name: "dist/", typ: tar.TypeDir}, {name: "dist/app", body: "bin"}}, packLimits{}, ""},
		{"dot slash prefix", []tarEntry{{name: "./dist/app", body: "bin"}}, packLimits{}, ""},
		{"inside symlink", []tarEntry{{name: "dist/app", body: "x"}, {name: "dist/link", typ: tar.TypeSymlink, target: "app"}}, packLimits{}, ""},
		{"inside hard link", []tarEntry{{name: "dist/app", body: "x"}, {name: "dist/h", typ: tar.TypeLink, target: "dist/app"}}, packLimits{}, ""},
		{"long but legal name", []tarEntry{{name: "dist/" + long, body: "x"}}, packLimits{}, ""},
		{"parent traversal", []tarEntry{{name: "../evil", body: "x"}}, packLimits{}, "leaves the workspace"},
		{"nested traversal", []tarEntry{{name: "dist/../../evil", body: "x"}}, packLimits{}, "leaves the workspace"},
		{"absolute name", []tarEntry{{name: "/etc/cron.d/x", body: "x"}}, packLimits{}, "leaves the workspace"},
		{"outside the requested path", []tarEntry{{name: "other/x", body: "x"}}, packLimits{}, "outside the requested path"},
		{"name too long", []tarEntry{{name: "dist/" + strings.Repeat("a", 5000), body: "x"}}, packLimits{}, "invalid"},
		{"symlink out", []tarEntry{{name: "dist/link", typ: tar.TypeSymlink, target: "../../etc/passwd"}}, packLimits{}, "points outside"},
		{"absolute symlink", []tarEntry{{name: "dist/link", typ: tar.TypeSymlink, target: "/etc/passwd"}}, packLimits{}, "points outside"},
		{"write through symlink", []tarEntry{{name: "dist", typ: tar.TypeSymlink, target: "."}, {name: "dist/b/c", body: "x"}}, packLimits{}, "through a symlink"},
		{"hard link out", []tarEntry{{name: "dist/h", typ: tar.TypeLink, target: "../secret"}}, packLimits{}, "points outside"},
		{"hard link absolute", []tarEntry{{name: "dist/h", typ: tar.TypeLink, target: "/etc/shadow"}}, packLimits{}, "points outside"},
		{"char device", []tarEntry{{name: "dist/dev", typ: tar.TypeChar}}, packLimits{}, "unsupported type"},
		{"block device", []tarEntry{{name: "dist/dev", typ: tar.TypeBlock}}, packLimits{}, "unsupported type"},
		{"fifo", []tarEntry{{name: "dist/pipe", typ: tar.TypeFifo}}, packLimits{}, "unsupported type"},
		{"gnu sparse", []tarEntry{{name: "dist/sparse", typ: tar.TypeGNUSparse}}, packLimits{}, "invalid tar header"},
		{"too many entries", many, packLimits{MaxEntries: 5}, "more than 5 files"},
		{"too big unpacked", []tarEntry{{name: "dist/big", body: strings.Repeat("a", 100)}}, packLimits{MaxUnpacked: 50}, "expands past"},
		{"secret inside", []tarEntry{{name: "dist/cfg", body: "token=s3cr3t-value\n"}}, packLimits{Secrets: []string{"s3cr3t-value"}}, "contains a secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := packFor(t, "dist", bytes.NewReader(dockerTar(t, tc.entries)), tc.lim)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("err = %v, want ok", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestPackerRebasesNestedPathAndNormalisesModes(t *testing.T) {
	t.Parallel()
	src := dockerTar(t, []tarEntry{
		{name: "app", body: "bin", mode: 0o4755},
		{name: "link", typ: tar.TypeSymlink, target: "app"},
	})
	if _, err := packFor(t, "out/bin/app", bytes.NewReader(src), packLimits{}); err == nil {
		t.Fatalf("entry %q outside the requested path was accepted", "link")
	}
	src = dockerTar(t, []tarEntry{{name: "app", body: "bin", mode: 0o4755}})
	hs, err := packFor(t, "out/bin/app", bytes.NewReader(src), packLimits{})
	if err != nil || len(hs) != 1 {
		t.Fatalf("pack = %v, %v", hs, err)
	}
	if hs[0].Name != "out/bin/app" {
		t.Errorf("name = %q, want out/bin/app", hs[0].Name)
	}
	if hs[0].Mode != 0o755 {
		t.Errorf("mode = %o, want setuid stripped to 755", hs[0].Mode)
	}
}

func TestPackerStatChecks(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	p := newPacker(tw, packLimits{})
	if err := p.add("dev", bytes.NewReader(nil), os.ModeDevice, ""); err == nil || !strings.Contains(err.Error(), "device") {
		t.Errorf("device stat = %v", err)
	}
	if err := p.add("l", bytes.NewReader(nil), os.ModeSymlink, "../../x"); err == nil || !strings.Contains(err.Error(), "points outside") {
		t.Errorf("escaping symlink stat = %v", err)
	}
	if err := p.add("empty", bytes.NewReader(dockerTar(t, nil)), 0, ""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty archive = %v", err)
	}
}

func TestPackerShortAndFailingStreams(t *testing.T) {
	t.Parallel()
	full := dockerTar(t, []tarEntry{{name: "dist/app", body: strings.Repeat("z", 4000)}})
	boom := errors.New("connection reset")
	tests := []struct {
		name string
		src  io.Reader
		want string
	}{
		{"cut mid entry", bytes.NewReader(full[:1500]), "unexpected EOF"},
		{"error mid entry", io.MultiReader(bytes.NewReader(full[:1200]), &errReader{boom}), "connection reset"},
		{"error before any byte", &errReader{boom}, "connection reset"},
	}
	for _, tc := range tests {
		_, err := packFor(t, "dist", tc.src, packLimits{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

func TestNeedleWriterAcrossWrites(t *testing.T) {
	t.Parallel()
	nw := newNeedleWriter([]string{"s3cr3t-value", ""})
	nw.reset("f")
	if _, err := nw.Write([]byte("xx s3cr3")); err != nil {
		t.Fatalf("partial write flagged: %v", err)
	}
	if _, err := nw.Write([]byte("t-value yy")); err == nil {
		t.Fatal("secret split across writes was not found")
	}
	nw.reset("g")
	if _, err := nw.Write([]byte("s3cr3t-valu")); err != nil {
		t.Fatal(err)
	}
	if _, err := nw.Write([]byte("X")); err != nil {
		t.Fatalf("reset entry kept the old tail: %v", err)
	}
}

func TestVerifiedReader(t *testing.T) {
	t.Parallel()
	s := newArtifactStoreT(t, ArtifactLimits{})
	w, _ := s.Begin("run", "a", 0)
	_, _ = w.Write([]byte("hello archive"))
	rel, size, sum, err := w.Commit()
	if err != nil {
		t.Fatal(err)
	}
	good := store.PipelineArtifact{StoragePath: rel, SizeBytes: size, SHA256: sum}
	tests := []struct {
		name    string
		meta    store.PipelineArtifact
		partial bool
		corrupt bool
	}{
		{"intact", good, false, false},
		{"intact, consumer stopped early", good, true, false},
		{"hash differs", store.PipelineArtifact{StoragePath: rel, SizeBytes: size, SHA256: strings.Repeat("0", 64)}, false, true},
		{"size differs", store.PipelineArtifact{StoragePath: rel, SizeBytes: size + 1, SHA256: sum}, false, true},
	}
	for _, tc := range tests {
		v, err := s.OpenVerified(tc.meta)
		if err != nil {
			t.Fatal(err)
		}
		var rerr error
		if tc.partial {
			_, rerr = v.Read(make([]byte, 3))
		} else {
			_, rerr = io.Copy(io.Discard, v)
		}
		verr := v.Verify()
		_ = v.Close()
		if tc.corrupt && !errors.Is(verr, ErrArtifactCorrupt) {
			t.Errorf("%s: verify = %v (read %v), want corrupt", tc.name, verr, rerr)
		}
		if !tc.corrupt && (verr != nil || rerr != nil) {
			t.Errorf("%s: verify = %v read = %v", tc.name, verr, rerr)
		}
	}
	if _, err := s.OpenVerified(store.PipelineArtifact{StoragePath: "../x"}); !errors.Is(err, ErrArtifactPath) {
		t.Errorf("escaping path = %v", err)
	}
}

func gzipOf(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(b)
	_ = gz.Close()
	return buf.Bytes()
}

func TestArtifactStoreLimitsAndIntegrity(t *testing.T) {
	t.Parallel()
	lim := ArtifactLimits{MaxBytes: 10, MaxRunBytes: 15, Retention: time.Hour}
	s, err := NewArtifactStore(t.TempDir(), lim)
	if err != nil {
		t.Fatal(err)
	}

	w, err := s.Begin("run1", "a1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("0123456789X")); !errors.Is(err, ErrArtifactTooLarge) {
		t.Errorf("write past MaxBytes = %v, want ErrArtifactTooLarge", err)
	}
	w.Abort()

	w, _ = s.Begin("run1", "a2", 10)
	if _, err := w.Write([]byte("123456")); !errors.Is(err, ErrArtifactTooLarge) {
		t.Errorf("write past the run budget = %v, want ErrArtifactTooLarge", err)
	}
	w.Abort()

	w, _ = s.Begin("run1", "a3", 0)
	if _, err := w.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	rel, size, sum, err := w.Commit()
	if err != nil || size != 10 {
		t.Fatalf("commit = %q %d %v", rel, size, err)
	}
	meta := store.PipelineArtifact{StoragePath: rel, SizeBytes: size, SHA256: sum}
	f, err := s.Open(meta)
	if err != nil {
		t.Fatalf("open intact artifact: %v", err)
	}
	_ = f.Close()

	bad := meta
	bad.SHA256 = strings.Repeat("0", 64)
	if _, err := s.Open(bad); !errors.Is(err, ErrArtifactCorrupt) {
		t.Errorf("hash mismatch = %v, want ErrArtifactCorrupt", err)
	}
	full, _ := s.resolve(rel)
	if err := os.WriteFile(full, []byte("tampered!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(meta); !errors.Is(err, ErrArtifactCorrupt) {
		t.Errorf("tampered file = %v, want ErrArtifactCorrupt", err)
	}
	for _, evil := range []string{"../x", "/etc/passwd", "run1/../../x", ""} {
		if _, err := s.Open(store.PipelineArtifact{StoragePath: evil}); !errors.Is(err, ErrArtifactPath) {
			t.Errorf("Open(%q) = %v, want ErrArtifactPath", evil, err)
		}
	}
}

type memIndex struct {
	rows []store.PipelineArtifact
}

func (m *memIndex) SavePipelineArtifact(_ context.Context, a store.PipelineArtifact) error {
	m.rows = append(m.rows, a)
	return nil
}

func (m *memIndex) ListPipelineArtifacts(context.Context, string) ([]store.PipelineArtifact, error) {
	return m.rows, nil
}

func (m *memIndex) ListExpiredPipelineArtifacts(_ context.Context, now time.Time) ([]store.PipelineArtifact, error) {
	var out []store.PipelineArtifact
	for _, r := range m.rows {
		if !r.ExpiresAt.After(now) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memIndex) DeletePipelineArtifact(_ context.Context, id string) error {
	for i, r := range m.rows {
		if r.ID == id {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			break
		}
	}
	return nil
}

func TestArtifactSweepRetentionAndOrphans(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := NewArtifactStore(dir, ArtifactLimits{Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.now = func() time.Time { return now }
	put := func(run, id string, expires time.Time) store.PipelineArtifact {
		w, _ := s.Begin(run, id, 0)
		_, _ = w.Write([]byte("data"))
		rel, size, sum, err := w.Commit()
		if err != nil {
			t.Fatal(err)
		}
		return store.PipelineArtifact{ID: id, RunID: run, StoragePath: rel, SizeBytes: size, SHA256: sum, ExpiresAt: expires}
	}
	idx := &memIndex{}
	idx.rows = []store.PipelineArtifact{put("runA", "old", now.Add(-time.Minute)), put("runA", "fresh", now.Add(time.Hour))}
	put("ghost", "orphan", now.Add(time.Hour))

	removed, err := s.Sweep(context.Background(), idx, func(_ context.Context, id string) bool { return id == "runA" })
	if err != nil || removed != 1 {
		t.Fatalf("sweep = %d, %v", removed, err)
	}
	if len(idx.rows) != 1 || idx.rows[0].ID != "fresh" {
		t.Errorf("rows after sweep = %+v", idx.rows)
	}
	if _, err := os.Stat(filepath.Join(dir, "ghost")); !os.IsNotExist(err) {
		t.Errorf("orphan run dir survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, idx.rows[0].StoragePath)); err != nil {
		t.Errorf("fresh artifact removed: %v", err)
	}
}
