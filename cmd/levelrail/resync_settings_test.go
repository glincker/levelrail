package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResyncFromSettingsFollowsLiveValue(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rs := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	src := resyncFromSettings(ctx, rs)
	if got := src(); got != 30*time.Second {
		t.Fatalf("default interval = %v, want 30s", got)
	}
	if _, err := rs.Set(ctx, runtimesettings.KeyReconcileResyncSeconds, json.RawMessage(`45`), "root"); err != nil {
		t.Fatal(err)
	}
	if got := src(); got != 45*time.Second {
		t.Fatalf("interval after save = %v, want 45s", got)
	}
}
