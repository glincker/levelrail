package sourceupload

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"sort"
	"testing"
)

type zipFile struct {
	name string
	body string
	mode fs.FileMode
}

func makeZip(t *testing.T, files []zipFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarNames(t *testing.T, tgz []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

func TestConvertZip(t *testing.T) {
	cases := []struct {
		name      string
		files     []zipFile
		lim       Limits
		wantErr   error
		wantNames []string
		wantIndex bool
		wantStrip string
	}{
		{
			name:      "flat site",
			files:     []zipFile{{name: "index.html", body: "hi"}, {name: "css/a.css", body: "a"}},
			wantNames: []string{"css/a.css", "index.html"},
			wantIndex: true,
		},
		{
			name:      "single wrapper folder",
			files:     []zipFile{{name: "site/index.html", body: "hi"}, {name: "site/img/a.png", body: "p"}},
			wantNames: []string{"img/a.png", "index.html"},
			wantIndex: true,
			wantStrip: "site",
		},
		{
			name:      "nested wrappers",
			files:     []zipFile{{name: "a/b/index.html", body: "hi"}, {name: "a/b/x.js", body: "x"}},
			wantNames: []string{"index.html", "x.js"},
			wantIndex: true,
			wantStrip: "a/b",
		},
		{
			name:      "macos metadata ignored",
			files:     []zipFile{{name: "site/index.html", body: "hi"}, {name: "__MACOSX/site/._index.html", body: "m"}, {name: "site/.DS_Store", body: "d"}},
			wantNames: []string{"index.html"},
			wantIndex: true,
			wantStrip: "site",
		},
		{
			name:      "no index keeps layout",
			files:     []zipFile{{name: "assets/a.css", body: "a"}, {name: "assets/b.css", body: "b"}},
			wantNames: []string{"assets/a.css", "assets/b.css"},
		},
		{
			name:    "zip slip",
			files:   []zipFile{{name: "index.html", body: "hi"}, {name: "../evil.txt", body: "x"}},
			wantErr: ErrUnsafeArchive,
		},
		{
			name:    "absolute path",
			files:   []zipFile{{name: "/etc/passwd", body: "x"}},
			wantErr: ErrUnsafeArchive,
		},
		{
			name:    "backslash path",
			files:   []zipFile{{name: "a\\..\\b.txt", body: "x"}},
			wantErr: ErrUnsafeArchive,
		},
		{
			name:    "symlink",
			files:   []zipFile{{name: "index.html", body: "hi"}, {name: "link", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777}},
			wantErr: ErrUnsafeArchive,
		},
		{
			name:    "duplicate entry",
			files:   []zipFile{{name: "index.html", body: "a"}, {name: "index.html", body: "b"}},
			wantErr: ErrUnsafeArchive,
		},
		{
			name:    "too many files",
			files:   []zipFile{{name: "a", body: "1"}, {name: "b", body: "2"}, {name: "c", body: "3"}},
			lim:     Limits{MaxFiles: 2},
			wantErr: ErrTooLarge,
		},
		{
			name:    "total bytes cap",
			files:   []zipFile{{name: "a", body: "123456"}, {name: "b", body: "123456"}},
			lim:     Limits{MaxBytes: 10},
			wantErr: ErrTooLarge,
		},
		{
			name:    "single file cap",
			files:   []zipFile{{name: "a", body: "123456"}},
			lim:     Limits{MaxFileBytes: 3},
			wantErr: ErrTooLarge,
		},
		{
			name:    "only junk",
			files:   []zipFile{{name: ".DS_Store", body: "d"}},
			wantErr: ErrEmptyArchive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zb := makeZip(t, tc.files)
			var out bytes.Buffer
			res, err := ConvertZip(bytes.NewReader(zb), int64(len(zb)), &out, tc.lim)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := tarNames(t, out.Bytes())
			if len(got) != len(tc.wantNames) {
				t.Fatalf("names = %v, want %v", got, tc.wantNames)
			}
			for i := range got {
				if got[i] != tc.wantNames[i] {
					t.Fatalf("names = %v, want %v", got, tc.wantNames)
				}
			}
			if res.HasIndex != tc.wantIndex || res.StrippedDir != tc.wantStrip || res.Files != len(tc.wantNames) {
				t.Fatalf("result = %+v", res)
			}
			if _, err := Inspect(bytes.NewReader(out.Bytes()), Limits{}); err != nil {
				t.Fatalf("converted archive fails Inspect: %v", err)
			}
		})
	}
}

func TestConvertZipNotAZip(t *testing.T) {
	data := []byte("definitely not a zip")
	if _, err := ConvertZip(bytes.NewReader(data), int64(len(data)), io.Discard, Limits{}); err == nil {
		t.Fatal("expected an error for non-zip input")
	}
}

func TestConvertZipDeterministic(t *testing.T) {
	zb := makeZip(t, []zipFile{{name: "b.txt", body: "b"}, {name: "index.html", body: "hi"}})
	var a, b bytes.Buffer
	if _, err := ConvertZip(bytes.NewReader(zb), int64(len(zb)), &a, Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ConvertZip(bytes.NewReader(zb), int64(len(zb)), &b, Limits{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("same zip produced different archives, dedupe by hash would break")
	}
}
