package mailbackup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseVersionRefusesNonMoxVersions(t *testing.T) {
	for _, in := range []string{"v0.0.17-go1.27.0\nlinux/arm64", "v0.0.17", "0.0.17+dirty"} {
		if _, s, err := parseVersion(in); err != nil || s != "v0.0.17" {
			t.Errorf("parseVersion(%q) = %q, %v", in, s, err)
		}
	}
	for _, in := range []string{"(devel)-go1.27.0", "go1.27.0", "", "mox v0.0.17"} {
		if _, _, err := parseVersion(in); err == nil {
			t.Errorf("parseVersion(%q) accepted a version that is not mox's own", in)
		}
	}
}

func TestPruneKeepsNewestMinKeepVerified(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		id := base.Add(time.Duration(i) * time.Hour).Format("20060102T150405Z")
		raw, _ := json.Marshal(Manifest{ID: id, CreatedAt: base.Add(time.Duration(i) * time.Hour), Verified: true})
		if err := os.WriteFile(filepath.Join(dir, manifestName(id)), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, archiveName(id)), make([]byte, 1000), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := &LocalSink{Dir: dir, CapBytes: 1, MinKeep: 3}
	removed, err := s.Prune(context.Background())
	if len(removed) != 2 {
		t.Fatalf("removed %v, want the 2 oldest", removed)
	}
	if err == nil {
		t.Fatal("the kept backups exceed the cap, want a CapError")
	}
	es, _ := s.entries()
	if len(es) != 3 {
		t.Fatalf("kept %d backups, want 3", len(es))
	}
	if n, err := MinKeepFromEnv(func(string) string { return "" }); err != nil || n != DefaultMinKeep {
		t.Fatalf("default min keep = %d, %v", n, err)
	}
}
