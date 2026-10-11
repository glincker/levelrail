package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// lastSchemaBeforeAuthLibrary is the newest migration v0.2.0-beta.18 shipped.
const lastSchemaBeforeAuthLibrary = 371

type legacyFixtureToken struct {
	id, raw, owner, agent string
	abilities             string
	expires, revoked      string
	lastUsed              string
}

const (
	fixtureAdminID  = "usr_admin"
	fixtureViewerID = "usr_viewer"
)

func legacyFixtureTokens() []legacyFixtureToken {
	future := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339Nano)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	return []legacyFixtureToken{
		{id: "tok_read", raw: "legacyrawreadtokenvalue0000000001", owner: fixtureAdminID, abilities: `["read"]`, lastUsed: "2026-08-01T10:00:00Z"},
		{id: "tok_root", raw: "legacyrawroottokenvalue0000000002", owner: fixtureAdminID, abilities: `["root"]`},
		{id: "tok_deploy_expiring", raw: "legacyrawdeploytokenvalue00000003", owner: fixtureAdminID, abilities: `["read","deploy"]`, expires: future},
		{id: "tok_agent", raw: "legacyrawagenttokenvalue000000004", owner: fixtureAdminID, abilities: `["read"]`, agent: "claude-desktop"},
		{id: "tok_viewer_root", raw: "legacyrawviewerroottoken000000005", owner: fixtureViewerID, abilities: `["root"]`},
		{id: "tok_expired", raw: "legacyrawexpiredtokenvalue0000006", owner: fixtureAdminID, abilities: `["read"]`, expires: past},
		{id: "tok_revoked", raw: "legacyrawrevokedtokenvalue0000007", owner: fixtureAdminID, abilities: `["root"]`, revoked: past},
		{id: "tok_orphan", raw: "legacyraworphantokenvalue00000008", owner: "usr_deleted", abilities: `["read"]`},
		{id: "tok_unknown_ability", raw: "legacyrawunknownability000000009", owner: fixtureAdminID, abilities: `["read","superuser"]`},
		{id: "tok_system", raw: "legacyrawsystemtokenvalue00000010", owner: "", abilities: `["root"]`},
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// openBeta18Fixture builds a database at the schema the last pre-library
// release shipped, seeds it like that release did, then upgrades it.
func openBeta18Fixture(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	old, err := store.Open(ctx, path, store.WithMigrationCeiling(lastSchemaBeforeAuthLibrary))
	if err != nil {
		t.Fatalf("open beta.18 schema: %v", err)
	}
	for _, u := range []struct{ id, email, abilities string }{
		{fixtureAdminID, "admin@example.com", `["root"]`},
		{fixtureViewerID, "viewer@example.com", `["read"]`},
	} {
		if _, err := old.ExecContext(ctx, `INSERT INTO users (id, email, display_name, password_hash, is_first_user, created_at, abilities) VALUES (?, ?, ?, '', 0, ?, ?)`,
			u.id, u.email, u.email, "2026-07-01T00:00:00Z", u.abilities); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	for _, tk := range legacyFixtureTokens() {
		if _, err := old.ExecContext(ctx, `
			INSERT INTO api_tokens (id, name, token_hash, abilities, created_at, last_used_at, expires_at, revoked_at, agent_name, owner_user_id)
			VALUES (?, ?, ?, ?, '2026-07-02T00:00:00Z', ?, ?, ?, ?, ?)`,
			tk.id, "name "+tk.id, hashToken(tk.raw), tk.abilities, nullable(tk.lastUsed), nullable(tk.expires), nullable(tk.revoked), tk.agent, tk.owner); err != nil {
			t.Fatalf("seed token %s: %v", tk.id, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade beta.18 schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func upgradedRouter(t *testing.T, db *store.DB) (*Router, *authengine.Engine) {
	t.Helper()
	eng, err := authengine.New(db.DB, authengine.Config{BaseURL: "http://upgrade.test", TokenPrefix: "lt", Directory: authengine.NewDirectory(db.DB)})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	return NewRouter(discardLogger(), testBrand(), db, WithAuthEngine(eng)), eng
}

func bearerStatus(rt *Router, path, token string) int {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	return rec.Code
}

func TestLegacyTokensSurviveUpgrade(t *testing.T) {
	type want struct {
		read, root int
		abilities  []string
	}
	expect := map[string]want{
		"tok_read":            {http.StatusOK, http.StatusForbidden, []string{AbilityRead}},
		"tok_root":            {http.StatusOK, http.StatusOK, []string{AbilityRoot}},
		"tok_deploy_expiring": {http.StatusOK, http.StatusForbidden, []string{AbilityRead, AbilityDeploy}},
		"tok_agent":           {http.StatusOK, http.StatusForbidden, []string{AbilityRead}},
		"tok_viewer_root":     {http.StatusForbidden, http.StatusForbidden, nil},
		"tok_expired":         {http.StatusUnauthorized, http.StatusUnauthorized, nil},
		"tok_revoked":         {http.StatusUnauthorized, http.StatusUnauthorized, nil},
		"tok_orphan":          {http.StatusUnauthorized, http.StatusUnauthorized, nil},
		"tok_unknown_ability": {http.StatusUnauthorized, http.StatusUnauthorized, nil},
		"tok_system":          {http.StatusOK, http.StatusOK, []string{AbilityRoot}},
	}
	for _, mode := range []string{"startup import", "first use"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db := openBeta18Fixture(t)
			rt, eng := upgradedRouter(t, db)
			if mode == "startup import" {
				sum, err := eng.ImportLegacyTokens(ctx)
				if err != nil || sum.Migrated != 5 || sum.Skipped != 4 || sum.Pending != 0 {
					t.Fatalf("import = %+v, %v; want 5 migrated, 4 skipped", sum, err)
				}
				again, err := eng.ImportLegacyTokens(ctx)
				if err != nil || again != (authengine.LegacyImportSummary{}) {
					t.Fatalf("second import = %+v, %v; want a no-op", again, err)
				}
			}
			for _, tk := range legacyFixtureTokens() {
				w := expect[tk.id]
				if got := bearerStatus(rt, "/api/v1/apps", tk.raw); got != w.read {
					t.Errorf("%s read route = %d, want %d", tk.id, got, w.read)
				}
				if got := bearerStatus(rt, "/api/v1/updates/settings", tk.raw); got != w.root {
					t.Errorf("%s root route = %d, want %d", tk.id, got, w.root)
				}
				if w.abilities == nil {
					continue
				}
				rec, err := rt.lookupBearerToken(ctx, tk.raw)
				if err != nil {
					t.Fatalf("%s lookup: %v", tk.id, err)
				}
				if rec.ID != tk.id || rec.OwnerUserID != tk.owner || !slices.Equal(rec.Abilities, w.abilities) || rec.AgentName != tk.agent {
					t.Errorf("%s record = %+v, want same id, owner, agent and abilities %v", tk.id, rec, w.abilities)
				}
			}
			assertImportedMetadata(t, eng)
			assertImportAudited(t, db)
			status, err := eng.LegacyImportStatus(ctx)
			if err != nil || status.Migrated != 5 || status.Pending != 0 {
				t.Fatalf("status = %+v, %v", status, err)
			}
		})
	}
}

func assertImportedMetadata(t *testing.T, eng *authengine.Engine) {
	t.Helper()
	recs, err := eng.ListTokens(context.Background(), fixtureAdminID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]authengine.TokenRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	read, ok := byID["tok_read"]
	if !ok || read.Name != "name tok_read" || read.LastUsedAt == nil || !read.CreatedAt.Equal(time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("tok_read = %+v, want name, created and last used carried over", read)
	}
	if exp := byID["tok_deploy_expiring"].ExpiresAt; exp == nil || time.Until(*exp) < 70*time.Hour {
		t.Fatalf("expiry not carried over: %v", exp)
	}
	if byID["tok_agent"].AgentName != "claude-desktop" {
		t.Fatalf("agent label lost: %+v", byID["tok_agent"])
	}
	for _, id := range []string{"tok_expired", "tok_revoked", "tok_unknown_ability"} {
		if _, ok := byID[id]; ok {
			t.Fatalf("%s must not be imported", id)
		}
	}
}

func assertImportAudited(t *testing.T, db *store.DB) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT path, actor_name FROM audit_log WHERE action = ?`, store.AuditActionLegacyTokenMigrated)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var path, actor string
		if err := rows.Scan(&path, &actor); err != nil {
			t.Fatal(err)
		}
		for _, tk := range legacyFixtureTokens() {
			if strings.Contains(path+actor, hashToken(tk.raw)) || strings.Contains(path+actor, tk.raw) {
				t.Fatalf("audit row leaks token material: %q", path)
			}
		}
		n++
	}
	if n != 5 {
		t.Fatalf("audit rows = %d, want 5", n)
	}
}

func TestLegacyTokenRevokedAfterImportStaysRevoked(t *testing.T) {
	ctx := context.Background()
	db := openBeta18Fixture(t)
	rt, eng := upgradedRouter(t, db)
	raw := legacyFixtureTokens()[0].raw
	if got := bearerStatus(rt, "/api/v1/apps", raw); got != http.StatusOK {
		t.Fatalf("first use = %d", got)
	}
	if err := rt.libraryRevokeToken(ctx, "tok_read"); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.ImportLegacyTokens(ctx); err != nil {
		t.Fatal(err)
	}
	if got := bearerStatus(rt, "/api/v1/apps", raw); got != http.StatusUnauthorized {
		t.Fatalf("revoked imported token = %d, want 401", got)
	}
}

func TestLegacyTokenLinkedButMissingIsRefused(t *testing.T) {
	ctx := context.Background()
	db := openBeta18Fixture(t)
	rt, _ := upgradedRouter(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO authengine_token_map (legacy_id, engine_id) VALUES ('tok_root', '01JZZZZZZZZZZZZZZZZZZZZZZZ')`); err != nil {
		t.Fatal(err)
	}
	if got := bearerStatus(rt, "/api/v1/apps", legacyFixtureTokens()[1].raw); got != http.StatusUnauthorized {
		t.Fatalf("token whose library row vanished = %d, want 401", got)
	}
}
