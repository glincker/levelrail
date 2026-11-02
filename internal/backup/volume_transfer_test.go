package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeTransferRuntime struct {
	listing   string
	exportOut string
	specs     []docker.ContainerSpec
	cmds      [][]string
	extracted []byte
	removed   int
}

func (f *fakeTransferRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.specs = append(f.specs, spec)
	return "helper", nil
}
func (f *fakeTransferRuntime) Start(context.Context, string) error { return nil }
func (f *fakeTransferRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	f.cmds = append(f.cmds, cmd)
	if cmd[0] == "tar" {
		return io.NopCloser(strings.NewReader(f.exportOut)), nil
	}
	if strings.HasPrefix(cmd[2], "ls") {
		return io.NopCloser(strings.NewReader(f.listing)), nil
	}
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeTransferRuntime) ExecWithInput(_ context.Context, _ string, cmd []string, in io.Reader) (io.ReadCloser, error) {
	f.cmds = append(f.cmds, cmd)
	f.extracted, _ = io.ReadAll(in)
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeTransferRuntime) Remove(context.Context, string, bool) error { f.removed++; return nil }

type tarEntry struct {
	name, link string
	typ        byte
	body       string
}

func buildArchive(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: typ, Mode: 0o644}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExportMountsReadOnlyAndCapsSize(t *testing.T) {
	f := &fakeTransferRuntime{exportOut: strings.Repeat("x", 100)}
	vt := &VolumeTransfer{Runtime: f}
	s, err := vt.Export(context.Background(), "data", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !f.specs[0].Volumes[0].ReadOnly {
		t.Fatal("export must mount read-only")
	}
	if b, err := io.ReadAll(s); err != nil || len(b) != 100 || s.Bytes() != 100 {
		t.Fatalf("read %d err %v", len(b), err)
	}
	_ = s.Close()
	if f.removed != 1 {
		t.Fatal("helper not removed")
	}

	f2 := &fakeTransferRuntime{exportOut: strings.Repeat("x", 100)}
	s2, _ := (&VolumeTransfer{Runtime: f2}).Export(context.Background(), "data", 10)
	if _, err := io.ReadAll(s2); !errors.Is(err, ErrTransferTooLarge) {
		t.Fatalf("want too large, got %v", err)
	}
}

func TestImportIntoEmptyVolume(t *testing.T) {
	f := &fakeTransferRuntime{}
	arc := buildArchive(t, []tarEntry{{name: "./", typ: tar.TypeDir}, {name: "a/", typ: tar.TypeDir}, {name: "a/f.txt", body: "hello"}, {name: "a/l", typ: tar.TypeSymlink, link: "f.txt"}})
	st, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(arc), ImportOptions{MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 3 {
		t.Fatalf("entries = %d", st.Entries)
	}
	names := []string{}
	tr := tar.NewReader(bytes.NewReader(f.extracted))
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "a,a/f.txt,a/l" {
		t.Fatalf("extracted %v", names)
	}
	if f.specs[0].Volumes[0].ReadOnly {
		t.Fatal("import must mount read-write")
	}
}

func TestImportRefusesPopulatedVolumeUnlessOverwrite(t *testing.T) {
	arc := buildArchive(t, []tarEntry{{name: "f", body: "x"}})
	f := &fakeTransferRuntime{listing: "old.txt\n"}
	_, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(arc), ImportOptions{MaxBytes: 1 << 20})
	if !errors.Is(err, ErrVolumeNotEmpty) || f.extracted != nil {
		t.Fatalf("want ErrVolumeNotEmpty, got %v", err)
	}
	f = &fakeTransferRuntime{listing: "old.txt\n"}
	if _, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(arc), ImportOptions{MaxBytes: 1 << 20, Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	wiped := false
	for _, c := range f.cmds {
		if len(c) > 2 && strings.Contains(c[2], "rm -rf") {
			wiped = true
		}
	}
	if !wiped {
		t.Fatal("overwrite must clear the volume first")
	}
}

func TestImportRejectsUnsafeEntries(t *testing.T) {
	cases := map[string][]tarEntry{
		"absolute":         {{name: "/etc/passwd", body: "x"}},
		"dotdot":           {{name: "a/../../b", body: "x"}},
		"symlink absolute": {{name: "l", typ: tar.TypeSymlink, link: "/etc"}},
		"symlink escape":   {{name: "d/l", typ: tar.TypeSymlink, link: "../../x"}},
		"through symlink":  {{name: "l", typ: tar.TypeSymlink, link: "d"}, {name: "l/f", body: "x"}},
		"hardlink escape":  {{name: "h", typ: tar.TypeLink, link: "../x"}},
		"device":           {{name: "n", typ: tar.TypeChar}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeTransferRuntime{listing: "old\n"}
			_, err := (&VolumeTransfer{Runtime: f}).Import(context.Background(), "data", bytes.NewReader(buildArchive(t, entries)), ImportOptions{MaxBytes: 1 << 20, Overwrite: true})
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("want ErrUnsafeArchive, got %v", err)
			}
			for _, c := range f.cmds {
				// Only the first entry is checked before the clear; later ones fail the stream.
				if name != "through symlink" && len(c) > 2 && strings.Contains(c[2], "rm -rf") {
					t.Fatal("an unsafe first entry must not wipe the volume")
				}
			}
		})
	}
}

func TestImportSizeCapAndNotGzip(t *testing.T) {
	arc := buildArchive(t, []tarEntry{{name: "big", body: strings.Repeat("x", 4096)}})
	_, err := (&VolumeTransfer{Runtime: &fakeTransferRuntime{}}).Import(context.Background(), "data", bytes.NewReader(arc), ImportOptions{MaxBytes: 1024})
	if !errors.Is(err, ErrTransferTooLarge) {
		t.Fatalf("want too large, got %v", err)
	}
	_, err = (&VolumeTransfer{Runtime: &fakeTransferRuntime{}}).Import(context.Background(), "data", strings.NewReader("plain text"), ImportOptions{MaxBytes: 1024})
	if err == nil || !strings.Contains(err.Error(), "not a gzip archive") {
		t.Fatalf("want gzip error, got %v", err)
	}
}
