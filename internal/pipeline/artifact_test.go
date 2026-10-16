package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
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
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Linkname: e.target}
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
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func collectOutput(archive []byte) string {
	enc := base64.StdEncoding.EncodeToString(archive)
	var sb strings.Builder
	sb.WriteString(collectBegin + "\n")
	for len(enc) > 76 {
		sb.WriteString(enc[:76] + "\n")
		enc = enc[76:]
	}
	sb.WriteString(enc + "\n" + collectEnd + "\n")
	return sb.String()
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
		{".", ".", false},
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

func TestValidateArchive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []tarEntry
		secrets []string
		max     int64
		wantErr string
	}{
		{"plain files", []tarEntry{{name: "dist/", typ: tar.TypeDir}, {name: "dist/app", body: "bin"}}, nil, 0, ""},
		{"dot slash prefix", []tarEntry{{name: "./dist/app", body: "bin"}}, nil, 0, ""},
		{"inside symlink", []tarEntry{{name: "dist/app", body: "x"}, {name: "dist/link", typ: tar.TypeSymlink, target: "app"}}, nil, 0, ""},
		{"parent traversal", []tarEntry{{name: "../evil", body: "x"}}, nil, 0, "leaves the workspace"},
		{"nested traversal", []tarEntry{{name: "a/../../evil", body: "x"}}, nil, 0, "leaves the workspace"},
		{"absolute name", []tarEntry{{name: "/etc/cron.d/x", body: "x"}}, nil, 0, "leaves the workspace"},
		{"symlink out", []tarEntry{{name: "dist/link", typ: tar.TypeSymlink, target: "../../etc/passwd"}}, nil, 0, "points outside"},
		{"absolute symlink", []tarEntry{{name: "link", typ: tar.TypeSymlink, target: "/etc/passwd"}}, nil, 0, "points outside"},
		{"write through symlink", []tarEntry{{name: "a", typ: tar.TypeSymlink, target: "."}, {name: "a/b/c", body: "x"}}, nil, 0, "through a symlink"},
		{"hard link out", []tarEntry{{name: "h", typ: tar.TypeLink, target: "../secret"}}, nil, 0, "points outside"},
		{"device node", []tarEntry{{name: "dev", typ: tar.TypeChar}}, nil, 0, "unsupported type"},
		{"too big unpacked", []tarEntry{{name: "big", body: strings.Repeat("a", 100)}}, nil, 50, "expands past"},
		{"secret inside", []tarEntry{{name: "cfg", body: "token=s3cr3t-value\n"}}, []string{"s3cr3t-value"}, 0, "contains a secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateArchive(bytes.NewReader(makeTarGz(t, tc.entries)), tc.secrets, tc.max)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
	if err := validateArchive(strings.NewReader("not gzip"), nil, 0); err == nil {
		t.Error("non-gzip data must be rejected")
	}
}

type chunkReader struct{ parts []string }

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.parts[0])
	c.parts = c.parts[1:]
	return n, nil
}

func TestContainsAnyAcrossChunks(t *testing.T) {
	t.Parallel()
	r := &chunkReader{parts: []string{"xx s3cr3", "t-value yy"}}
	found, err := containsAny(r, []string{"s3cr3t-value"})
	if err != nil || !found {
		t.Fatalf("split secret not found: %v %v", found, err)
	}
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
