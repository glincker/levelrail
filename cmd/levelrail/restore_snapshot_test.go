package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedSnapshot(t *testing.T, dataDir string) (string, cpbackup.Info) {
	t.Helper()
	ctx := context.Background()
	live := filepath.Join(dataDir, storeFilename)
	db, err := store.Open(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	info, err := cpbackup.NewManager(db, dataDir).Create(ctx)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	_ = db.Close()
	return live, info
}

func TestRestoreSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		args      func(info cpbackup.Info) []string
		stdin     string
		wantErr   string
		wantOut   string
		wantAside bool
	}{
		{"list", func(cpbackup.Info) []string { return []string{"--list"} }, "", "", "levelrail-", false},
		{"dry run changes nothing", func(cpbackup.Info) []string { return []string{"--dry-run", "latest"} }, "", "", "dry run", false},
		{"declined prompt aborts", func(cpbackup.Info) []string { return []string{"latest"} }, "no\n", "aborted", "", false},
		{"unknown name", func(cpbackup.Info) []string { return []string{"nope.db"} }, "", "no local snapshot named", "", false},
		{"confirmed restore", func(cpbackup.Info) []string { return []string{"latest"} }, "yes\n", "", "restored", true},
		{"yes flag restores by name", func(i cpbackup.Info) []string { return []string{"--yes", i.Name} }, "", "", "restored", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			live, info := seedSnapshot(t, dataDir)
			var out bytes.Buffer
			err := runRestoreSnapshot(context.Background(), tc.args(info), dataDir, strings.NewReader(tc.stdin), &out)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output %q missing %q", out.String(), tc.wantOut)
			}
			matches, _ := filepath.Glob(live + ".before-restore-*")
			if (len(matches) == 1) != tc.wantAside {
				t.Errorf("before-restore copies = %v, want aside=%v", matches, tc.wantAside)
			}
		})
	}
}

func TestRestoreSnapshotNoSnapshots(t *testing.T) {
	err := runRestoreSnapshot(context.Background(), []string{"latest"}, t.TempDir(), strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no local snapshots") {
		t.Fatalf("err = %v", err)
	}
}
