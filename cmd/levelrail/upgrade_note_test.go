package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
)

func TestParseUpgradeNoteFlags(t *testing.T) {
	long := strings.Repeat("a", upgradeNoteMaxByLen+1)
	tests := []struct {
		name       string
		args       []string
		wantErr    string
		wantMethod string
	}{
		{"minimal", []string{"--by", "gagan"}, "", upgradehistory.MethodManual},
		{"package method", []string{"--by", "gagan", "--method", "package"}, "", upgradehistory.MethodPackage},
		{"ci method with reason", []string{"--by", "ci-bot", "--method", "ci", "--reason", "hotfix"}, "", upgradehistory.MethodCI},
		{"missing by", []string{"--method", "ci"}, "--by is required", ""},
		{"blank by", []string{"--by", "   "}, "--by is required", ""},
		{"by too long", []string{"--by", long}, "--by must be at most", ""},
		{"by control char", []string{"--by", "a\nb"}, "control characters", ""},
		{"bad method", []string{"--by", "x", "--method", "rollback"}, "--method must be", ""},
		{"reason too long", []string{"--by", "x", "--reason", strings.Repeat("r", upgradeNoteMaxReason+1)}, "--reason must be at most", ""},
		{"unknown flag", []string{"--by", "x", "--nope"}, "usage:", ""},
		{"stray argument", []string{"--by", "x", "extra"}, "usage:", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseUpgradeNoteFlags(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.method != tc.wantMethod || f.dataDir == "" {
				t.Fatalf("flags = %+v", f)
			}
		})
	}
}

func TestRunUpgradeNoteWritesMarker(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	args := []string{"--by", "gagan", "--method", "package", "--reason", "apt upgrade", "--data-dir", dir}
	if err := runUpgradeNote(args, &out, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, upgradehistory.MarkerFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("marker perms %o, want 600", perm)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var m upgradehistory.Marker
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := upgradehistory.Marker{
		ToVersion: version.Version, Initiator: "gagan", Method: upgradehistory.MethodPackage,
		Reason: "apt upgrade", WrittenAt: now,
	}
	if m != want {
		t.Fatalf("marker = %+v, want %+v", m, want)
	}
	if got, ok := upgradehistory.ConsumeMarker(dir, version.Version, now); !ok || got.Initiator != "gagan" {
		t.Fatalf("next boot did not consume the marker: %+v %v", got, ok)
	}
	if _, ok := upgradehistory.ConsumeMarker(dir, version.Version, now); ok {
		t.Fatal("marker consumed twice")
	}
}

func TestRunUpgradeNoteMissingDataDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	err := runUpgradeNote([]string{"--by", "x", "--data-dir", missing}, &bytes.Buffer{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatal("must not create the data dir")
	}
}
