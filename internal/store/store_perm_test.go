package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRestrictsDatabaseFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil { //nolint:gosec // deliberately loose to prove Open tightens it
		t.Fatal(err)
	}
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, p := range []string{path, path + "-wal"} {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s mode = %v, want no group or world access", p, fi.Mode().Perm())
		}
	}
}
