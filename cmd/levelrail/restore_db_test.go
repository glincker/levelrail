package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunRestoreDB(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	live := filepath.Join(dataDir, storeFilename)

	db, err := store.Open(ctx, live)
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(t.TempDir(), "snap.db")
	if _, _, err := db.SnapshotTo(ctx, snap); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	var out bytes.Buffer
	if err := runRestoreDB(ctx, []string{snap}, dataDir, &out); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(out.String(), "WARNING: stop the control plane") {
		t.Errorf("missing stop warning: %q", out.String())
	}
	matches, _ := filepath.Glob(live + ".before-restore-*")
	if len(matches) != 1 {
		t.Fatalf("before-restore copies = %v, want 1", matches)
	}
	if _, err := store.InspectSnapshot(ctx, live); err != nil {
		t.Errorf("restored db invalid: %v", err)
	}
}

func TestRunRestoreDB_RejectsBadFile(t *testing.T) {
	dataDir := t.TempDir()
	bad := filepath.Join(t.TempDir(), "bad.db")
	if err := os.WriteFile(bad, []byte("garbage garbage garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runRestoreDB(context.Background(), []string{bad}, dataDir, &bytes.Buffer{}); err == nil {
		t.Fatal("expected rejection")
	}
	if _, err := os.Stat(filepath.Join(dataDir, storeFilename)); err == nil {
		t.Error("live database must not be created or touched on rejection")
	}
}
