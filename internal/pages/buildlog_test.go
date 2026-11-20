package pages

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowKeepsTail(t *testing.T) {
	lines := []string{"aaaa", "bbbb", "cccc"}
	got, cut := Window(lines, func(s string) int { return len(s) }, 10)
	if !cut || len(got) != 2 || got[0] != "bbbb" {
		t.Fatalf("got %v cut=%v", got, cut)
	}
	if _, cut := Window(lines, func(s string) int { return len(s) }, 100); cut {
		t.Fatal("should not cut")
	}
	if !LogExpired(time.Now().Add(-2*time.Hour), time.Now(), time.Hour) {
		t.Fatal("expected expiry")
	}
}

func TestEvictOldestSparesKeyAndSite(t *testing.T) {
	root := t.TempDir()
	dir, err := CacheDir(root, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CacheDir(root, "../x"); err == nil {
		t.Fatal("expected invalid site")
	}
	for i, k := range []string{"old", "new"} {
		p := filepath.Join(dir, k)
		_ = os.MkdirAll(p, 0o700)
		_ = os.WriteFile(filepath.Join(p, "f"), make([]byte, 100), 0o600)
		ts := time.Now().Add(time.Duration(i-5) * time.Hour)
		_ = os.Chtimes(p, ts, ts)
	}
	if err := EvictOldest(dir, "new", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old")); !os.IsNotExist(err) {
		t.Fatal("old should be evicted")
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); err != nil {
		t.Fatal("new should stay")
	}
	if CacheKey([]byte("a")) == CacheKey([]byte("b")) {
		t.Fatal("keys must differ")
	}
}
