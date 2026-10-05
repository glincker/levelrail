package ingress

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestImportFileCerts(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	files := map[string]string{
		"certificates/ca/example.com/example.com.crt": "crt",
		"certificates/ca/example.com/example.com.key": "key",
		"acme/ca/users/a@b.c/a.json":                  "acct",
		"locks/x.lock":                                "lock",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveCertStorageValue(ctx, "certificates/ca/example.com/example.com.key", []byte("keep")); err != nil {
		t.Fatal(err)
	}

	n, err := ImportFileCerts(ctx, db, dir, nil)
	if err != nil || n != 2 {
		t.Fatalf("first import = %d, %v; want 2 keys", n, err)
	}
	got, err := db.GetCertStorageValue(ctx, "certificates/ca/example.com/example.com.key")
	if err != nil || string(got.Value) != "keep" {
		t.Fatalf("existing key overwritten: %+v %v", got, err)
	}
	if n, err = ImportFileCerts(ctx, db, dir, nil); err != nil || n != 0 {
		t.Fatalf("second import = %d, %v; want idempotent 0", n, err)
	}
	if n, err = ImportFileCerts(ctx, db, filepath.Join(dir, "missing"), nil); err != nil || n != 0 {
		t.Fatalf("missing dir = %d, %v", n, err)
	}
}
