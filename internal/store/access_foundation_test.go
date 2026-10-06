package store

import (
	"context"
	"strings"
	"testing"
)

// migrationStatement returns the first statement in the embedded migration
// that starts with prefix, so a test runs the SQL that actually ships.
func migrationStatement(t *testing.T, version int, prefix string) string {
	t.Helper()
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	for _, m := range migrations {
		if m.version != version {
			continue
		}
		for _, stmt := range strings.Split(m.sql, ";") {
			stmt = strings.TrimSpace(stmt)
			// Skip comment lines that precede a statement.
			var lines []string
			for _, l := range strings.Split(stmt, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(l), "--") {
					lines = append(lines, l)
				}
			}
			stmt = strings.TrimSpace(strings.Join(lines, "\n"))
			if strings.HasPrefix(stmt, prefix) {
				return stmt
			}
		}
	}
	t.Fatalf("no statement starting %q in migration %04d", prefix, version)
	return ""
}

func scalar(t *testing.T, db *DB, query string) string {
	t.Helper()
	var out string
	if err := db.QueryRowContext(context.Background(), query).Scan(&out); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return out
}

func TestAccessFoundation_FreshInstallSeeds(t *testing.T) {
	db := openTestDB(t)

	roles := map[string]string{}
	rows, err := db.QueryContext(context.Background(), `SELECT name, visibility FROM roles WHERE builtin = 1`)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, vis string
		if err := rows.Scan(&name, &vis); err != nil {
			t.Fatal(err)
		}
		roles[name] = vis
	}
	want := map[string]string{"admin": "all", "operator": "all", "viewer": "all", "guest": "granted"}
	for name, vis := range want {
		if roles[name] != vis {
			t.Errorf("role %q visibility = %q, want %q", name, roles[name], vis)
		}
	}
	if got := scalar(t, db, `SELECT abilities FROM roles WHERE name = 'guest'`); got != `["read"]` {
		t.Errorf("guest abilities = %s, want read only", got)
	}

	envs := map[string]string{}
	erows, err := db.QueryContext(context.Background(), `SELECT kind, protected FROM environments WHERE scope = 'global'`)
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	defer func() { _ = erows.Close() }()
	for erows.Next() {
		var kind, protected string
		if err := erows.Scan(&kind, &protected); err != nil {
			t.Fatal(err)
		}
		envs[kind] = protected
	}
	for _, kind := range []string{"dev", "test", "uat", "production"} {
		if _, ok := envs[kind]; !ok {
			t.Errorf("global environment of kind %q was not seeded", kind)
		}
	}
	if envs["production"] != "1" || envs["dev"] != "0" {
		t.Errorf("protected flags = %v, want only production protected", envs)
	}

	if got := scalar(t, db, `SELECT mode FROM ai_control_settings WHERE id = 1`); got != "off" {
		t.Errorf("fresh AI mode = %q, want off", got)
	}
}

func TestAccessFoundation_ExistingAgentTokensKeepAIWorking(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	update := migrationStatement(t, 376, "UPDATE ai_control_settings")

	insertToken := func(id, agent string, revoked bool) {
		t.Helper()
		revokedAt := "NULL"
		if revoked {
			revokedAt = "'2026-10-01T00:00:00Z'"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO api_tokens (id, name, token_hash, abilities, created_at, agent_name, revoked_at)
			VALUES ('`+id+`', 'n', 'h-`+id+`', '["read"]', '2026-10-01T00:00:00Z', '`+agent+`', `+revokedAt+`)`); err != nil {
			t.Fatalf("insert token: %v", err)
		}
	}
	reset := func() {
		t.Helper()
		if _, err := db.ExecContext(ctx, `UPDATE ai_control_settings SET mode = 'off'`); err != nil {
			t.Fatal(err)
		}
	}

	insertToken("tok_plain", "", false)
	insertToken("tok_revoked_agent", "bot", true)
	reset()
	if _, err := db.ExecContext(ctx, update); err != nil {
		t.Fatalf("run migration update: %v", err)
	}
	if got := scalar(t, db, `SELECT mode FROM ai_control_settings`); got != "off" {
		t.Errorf("mode = %q with only plain and revoked tokens, want off", got)
	}

	insertToken("tok_agent", "bot", false)
	reset()
	if _, err := db.ExecContext(ctx, update); err != nil {
		t.Fatalf("run migration update: %v", err)
	}
	if got := scalar(t, db, `SELECT mode FROM ai_control_settings`); got != "operate" {
		t.Errorf("mode = %q with a live agent token, want operate so it keeps working", got)
	}
}

func TestAccessFoundation_PreviewEnvironmentsAreRetagged(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	retag := migrationStatement(t, 375, "UPDATE environments SET kind = 'preview'")

	if _, err := db.ExecContext(ctx, `INSERT INTO projects (id, name) VALUES ('preview-shop', 'preview-shop')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"preview-env-shop", "env_qa"} {
		name := "Staging"
		if strings.HasPrefix(id, "preview-env-") {
			name = "Preview"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO environments (id, project_id, name) VALUES (?, 'preview-shop', ?)`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, retag); err != nil {
		t.Fatalf("run retag: %v", err)
	}
	if got := scalar(t, db, `SELECT kind FROM environments WHERE id = 'preview-env-shop'`); got != "preview" {
		t.Errorf("dynamic preview environment kind = %q, want preview", got)
	}
	if got := scalar(t, db, `SELECT kind FROM environments WHERE id = 'env_qa'`); got != "custom" {
		t.Errorf("ordinary environment kind = %q, want custom (untouched)", got)
	}
}

func TestAccessFoundation_GuestGrantsFollowTheUserAndEnvironment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, display_name, created_at) VALUES ('u1', 'g@example.com', 'G', '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_environment_grants (user_id, environment_id) VALUES ('u1', 'env_dev')`); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_environment_grants (user_id, environment_id) VALUES ('u1', 'env_dev')`); err == nil {
		t.Error("granting the same environment twice must fail on the primary key")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = 'u1'`); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM user_environment_grants`); got != "0" {
		t.Errorf("grants after deleting the user = %s, want 0 (cascade)", got)
	}
}
