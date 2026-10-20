package build

import (
	"archive/tar"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func FuzzUntarContextStaysInsideDir(f *testing.F) {
	seed := func(name string, typ byte, link string) []byte {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: typ, Linkname: link, Mode: 0o644, Size: 0})
		_ = tw.Close()
		return buf.Bytes()
	}
	f.Add(seed("ok.txt", tar.TypeReg, ""))
	f.Add(seed("../escape", tar.TypeReg, ""))
	f.Add(seed("/abs/path", tar.TypeReg, ""))
	f.Add(seed("a/../../b", tar.TypeReg, ""))
	f.Add(seed("link", tar.TypeSymlink, "../../etc"))
	f.Add(seed("link", tar.TypeSymlink, "/etc"))
	f.Add([]byte("not a tar"))

	f.Fuzz(func(t *testing.T, data []byte) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "ctx")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		_ = UntarContext(context.Background(), bytes.NewReader(data), dir)

		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("extraction wrote outside its directory: %v", entries)
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.Type()&fs.ModeSymlink == 0 {
				return nil
			}
			target, _ := os.Readlink(p)
			if filepath.IsAbs(target) {
				t.Errorf("absolute symlink target %q was created", target)
			}
			return nil
		})
	})
}
