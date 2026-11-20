package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration3278_PinNameBecomesNodeID(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3277))
	if err != nil {
		t.Fatalf("open at 3277: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO nodes (id, name) VALUES ('0123456789abcdef01234567', 'edge-1')`); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	for app, node := range map[string]string{"byname": "edge-1", "byid": "0123456789abcdef01234567", "ghost": "gone"} {
		if _, err := old.ExecContext(ctx, `INSERT INTO app_build_runner (app_name, node_id, pinned, updated_at) VALUES (?, ?, 1, 'x')`, app, node); err != nil {
			t.Fatalf("seed pin %s: %v", app, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()
	want := map[string]string{"byname": "0123456789abcdef01234567", "byid": "0123456789abcdef01234567", "ghost": "gone"}
	for app, id := range want {
		p, ok, err := cur.GetBuildRunnerPref(ctx, app)
		if err != nil || !ok || p.NodeID != id {
			t.Errorf("%s: node_id %q ok=%v err=%v, want %q", app, p.NodeID, ok, err, id)
		}
	}
}
