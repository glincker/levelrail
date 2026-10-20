package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration0960_RetiresAssistantRootToken(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(950))
	if err != nil {
		t.Fatalf("open at 950: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO api_tokens (id, name, token_hash, abilities, owner_user_id) VALUES ('t1', 'AI Assistant (internal)', 'h1', '["root"]', '')`,
		`INSERT INTO api_tokens (id, name, token_hash, abilities, owner_user_id) VALUES ('t2', 'AI Assistant (internal)', 'h2', '["root"]', 'u1')`,
		`INSERT INTO service_secrets (service_name, wrapped_dek) VALUES ('ai-assistant-internal-token', x'00'), ('ai-assistant', x'00')`,
		`INSERT INTO service_secret_values (service_name, env_key, ciphertext) VALUES ('ai-assistant-internal-token', 'token', x'00'), ('ai-assistant', 'api_key', x'00')`,
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

	var revoked1, revoked2 *string
	if err := cur.QueryRowContext(ctx, `SELECT revoked_at FROM api_tokens WHERE id = 't1'`).Scan(&revoked1); err != nil {
		t.Fatal(err)
	}
	if err := cur.QueryRowContext(ctx, `SELECT revoked_at FROM api_tokens WHERE id = 't2'`).Scan(&revoked2); err != nil {
		t.Fatal(err)
	}
	if revoked1 == nil {
		t.Error("owner-less assistant token still live")
	}
	if revoked2 != nil {
		t.Error("owned token with the same name was revoked")
	}
	var n int
	if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_secret_values WHERE service_name = 'ai-assistant-internal-token'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("retired token secret rows = %d, err = %v, want 0", n, err)
	}
	if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_secret_values WHERE service_name = 'ai-assistant'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("provider key rows = %d, err = %v, want 1", n, err)
	}
}
