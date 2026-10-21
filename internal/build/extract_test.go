package build

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func tarOf(t *testing.T, entries []tar.Header, bodies []string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i, h := range entries {
		h.Size = int64(len(bodies[i]))
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(bodies[i])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestUntarConfined(t *testing.T) {
	tests := []struct {
		name     string
		entries  []tar.Header
		bodies   []string
		max      int64
		wantErr  error
		wantFile string
		absent   string
	}{
		{
			name:     "files and dirs",
			entries:  []tar.Header{{Name: "./", Typeflag: tar.TypeDir}, {Name: "./assets/", Typeflag: tar.TypeDir}, {Name: "./assets/a.js", Typeflag: tar.TypeReg}},
			bodies:   []string{"", "", "x"},
			wantFile: "assets/a.js",
		},
		{
			name:    "escape skipped",
			entries: []tar.Header{{Name: "../evil", Typeflag: tar.TypeReg}, {Name: "ok.txt", Typeflag: tar.TypeReg}},
			bodies:  []string{"x", "y"}, wantFile: "ok.txt", absent: "../evil",
		},
		{
			name:    "symlink skipped",
			entries: []tar.Header{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}},
			bodies:  []string{""}, absent: "link",
		},
		{
			name:    "over cap",
			entries: []tar.Header{{Name: "big", Typeflag: tar.TypeReg}},
			bodies:  []string{"0123456789"}, max: 4, wantErr: ErrExtractTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "out")
			err := UntarConfined(tarOf(t, tt.entries, tt.bodies), dest, tt.max)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantFile != "" {
				if _, err := os.Stat(filepath.Join(dest, tt.wantFile)); err != nil {
					t.Fatalf("missing %s: %v", tt.wantFile, err)
				}
			}
			if tt.absent != "" {
				if _, err := os.Lstat(filepath.Join(dest, tt.absent)); err == nil {
					t.Fatalf("%s should not exist", tt.absent)
				}
			}
		})
	}
}

func TestStaticMaxBytesFromEnv(t *testing.T) {
	tests := []struct {
		val  string
		want int64
	}{{"", 2048 << 20}, {"10", 10 << 20}, {"0", 0}, {"junk", 2048 << 20}}
	for _, tt := range tests {
		got := StaticMaxBytesFromEnv(func(string) (string, bool) { return tt.val, true })
		if got != tt.want {
			t.Errorf("%q: got %d want %d", tt.val, got, tt.want)
		}
	}
}
