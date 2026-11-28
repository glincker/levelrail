package cpbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func writeIdentityDB(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE instance_identity (id INTEGER PRIMARY KEY, instance_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO instance_identity (id, instance_id) VALUES (1, ?)`, id); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstanceCheckShowsPairAndConfirmHint(t *testing.T) {
	restored := writeIdentityDB(t, "inst_backup")
	live := writeIdentityDB(t, "inst_live")
	c := instanceCheck(context.Background(), Manifest{}, restored, live)
	if !strings.Contains(c.Detail, "inst_backup") || !strings.Contains(c.Detail, "inst_live") || !strings.Contains(c.Detail, "same instance") {
		t.Fatalf("detail = %q", c.Detail)
	}
	same := instanceCheck(context.Background(), Manifest{InstanceID: "inst_live"}, restored, live)
	if strings.Contains(same.Detail, "same instance") {
		t.Fatalf("matching ids must not ask for confirmation: %q", same.Detail)
	}
}

func TestManifestInstanceIDIsOptional(t *testing.T) {
	raw, err := json.Marshal(Manifest{InstallID: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "instance_id") {
		t.Fatalf("empty instance id must be omitted: %s", raw)
	}
}
