package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadIaCInputSplitsComposeRefs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "compose"), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  compose: {path: compose/shop.yaml}\n"
	for name, body := range map[string]string{"app.yaml": doc, "compose/shop.yaml": "services: {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, att, err := readIaCInput([]string{dir})
	if err != nil || len(files) != 1 || att["compose/shop.yaml"] != "services: {}\n" {
		t.Fatalf("files=%v att=%v err=%v", files, att, err)
	}
	if err := os.Remove(filepath.Join(dir, "compose/shop.yaml")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "compose/shop.yaml")); err != nil {
		t.Skip("no symlinks here")
	}
	if _, _, err := readIaCInput([]string{filepath.Join(dir, "app.yaml")}); err == nil {
		t.Fatal("a symlinked compose file must be refused")
	}
}
