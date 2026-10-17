package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration0970_MarksControlPlaneKeysForReenroll(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(960))
	if err != nil {
		t.Fatalf("open at 960: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO nodes (id, name, cert_fingerprint, cert_key_origin) VALUES ('legacy', 'legacy', 'fp1', 'server')`,
		`INSERT INTO nodes (id, name, cert_fingerprint, cert_key_origin) VALUES ('modern', 'modern', 'fp2', 'agent')`,
		`INSERT INTO nodes (id, name, cert_fingerprint, cert_key_origin) VALUES ('nocert', 'nocert', '', 'server')`,
	} {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
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
	for id, want := range map[string]string{"legacy": CertKeyOriginReenrollRequired, "modern": CertKeyOriginAgent, "nocert": ""} {
		var got string
		if err := cur.QueryRowContext(ctx, `SELECT cert_key_origin FROM nodes WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("node %s key origin = %q, want %q", id, got, want)
		}
	}
}
