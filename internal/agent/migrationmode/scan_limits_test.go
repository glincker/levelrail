package migrationmode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
)

func bindInventory(sources ...string) *migrationpb.Inventory {
	var mounts []*migrationpb.Mount
	for _, s := range sources {
		mounts = append(mounts, &migrationpb.Mount{Type: "bind", Source: s, Destination: "/x"})
	}
	return &migrationpb.Inventory{Containers: []*migrationpb.Container{{Name: "c", Mounts: mounts}}}
}

func TestLearnSkipsForbiddenBindSources(t *testing.T) {
	ok := t.TempDir()
	cases := []struct {
		source string
		learn  bool
	}{
		{"/", false},
		{"/proc", false},
		{"/proc/1/root", false},
		{"/sys/kernel", false},
		{"/dev", false},
		{"/dev/shm/x", false},
		{"/procfs-data", true},
		{"/devices", true},
		{"/srv/data/../../", false},
		{ok, true},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			s := NewScanner()
			s.Learn(bindInventory(tc.source))
			_, err := s.root(filepath.Clean(tc.source))
			if (err == nil) != tc.learn {
				t.Fatalf("learned=%v want %v", err == nil, tc.learn)
			}
		})
	}
}

func TestScanRefusesTooManyTargets(t *testing.T) {
	dir := t.TempDir()
	s := NewScannerWithLimits(ScanLimits{MaxTargets: 2})
	s.Learn(bindInventory(dir))
	res := s.Scan(context.Background(), "r", []string{dir, dir, dir})
	if len(res.Stats) != 1 || !strings.Contains(res.Stats[0].Error, "too many") {
		t.Fatalf("unexpected result: %+v", res.Stats)
	}
	if res := s.Scan(context.Background(), "r", []string{dir, dir}); res.Stats[0].Error != "" {
		t.Fatalf("a request at the limit must be served: %+v", res.Stats)
	}
}

func TestScanEntryAndDepthLimits(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"f1", "f2", "f3"} {
		if err := os.WriteFile(filepath.Join(deep, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name    string
		lim     ScanLimits
		wantErr bool
	}{
		{"unlimited", ScanLimits{}, false},
		{"entries within", ScanLimits{MaxEntries: 100}, false},
		{"entries exceeded", ScanLimits{MaxEntries: 3}, true},
		{"depth within", ScanLimits{MaxDepth: 4}, false},
		{"depth exceeded", ScanLimits{MaxDepth: 2}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScannerWithLimits(tc.lim)
			s.Learn(bindInventory(dir))
			res := s.Scan(context.Background(), "r", []string{dir})
			if got := res.Stats[0].Error != ""; got != tc.wantErr {
				t.Fatalf("error=%q wantErr=%v", res.Stats[0].Error, tc.wantErr)
			}
		})
	}
}

func TestSaveIdentityTightensExistingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // the test needs a loose pre-existing dir
		t.Fatal(err)
	}
	if err := SaveIdentity(filepath.Join(dir, "identity.json"), &Identity{SessionID: "ms_x"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode %v, want 0700", info.Mode().Perm())
	}
}
