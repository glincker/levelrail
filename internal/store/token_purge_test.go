package store

import (
	"context"
	"testing"
	"time"
)

type purgeSeed struct {
	id      string
	revoked *time.Time
	expires *time.Time
	mapped  bool
}

func seedPurgeToken(t *testing.T, db *DB, s purgeSeed) {
	t.Helper()
	ctx := context.Background()
	created := time.Now().Add(-1000 * 24 * time.Hour)
	if err := db.SaveAPIToken(ctx, APIToken{ID: s.id, Name: s.id, TokenHash: "h-" + s.id, CreatedAt: created, ExpiresAt: s.expires}); err != nil {
		t.Fatalf("seed mirror %s: %v", s.id, err)
	}
	if s.revoked != nil {
		if _, err := db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ?`, formatTime(*s.revoked), s.id); err != nil {
			t.Fatalf("revoke mirror %s: %v", s.id, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO api_token_scopes (token_id, team_id, role) VALUES (?, 't', 'viewer')`, s.id); err != nil {
		t.Fatalf("seed scope %s: %v", s.id, err)
	}
	if !s.mapped {
		return
	}
	var rev, exp any
	if s.revoked != nil {
		rev = s.revoked.UnixMicro()
	}
	if s.expires != nil {
		exp = s.expires.UnixMicro()
	}
	eid := "eng-" + s.id
	if _, err := db.ExecContext(ctx, `INSERT INTO theauth_api_tokens (id, owner_id, owner_kind, name, token_hash, created_at, expires_at, revoked_at)
		VALUES (?, 'o', 'user', ?, ?, ?, ?, ?)`, eid, s.id, []byte("h-"+s.id), created.UnixMicro(), exp, rev); err != nil {
		t.Fatalf("seed library %s: %v", s.id, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO authengine_token_map (legacy_id, engine_id) VALUES (?, ?)`, s.id, eid); err != nil {
		t.Fatalf("seed map %s: %v", s.id, err)
	}
}

func countRows(t *testing.T, db *DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestPurgeInactiveAPITokens(t *testing.T) {
	now := time.Now()
	ago := func(days int, extra time.Duration) *time.Time {
		v := now.Add(-time.Duration(days)*24*time.Hour - extra)
		return &v
	}
	future := ago(-30, 0)
	cases := []struct {
		name    string
		kind    TokenPurgeKind
		cutoff  time.Time
		limit   int
		dryRun  bool
		seeds   []purgeSeed
		want    int
		wantTok int
	}{
		{name: "active never deleted", kind: TokenPurgeRevoked, cutoff: now, limit: 10,
			seeds: []purgeSeed{{id: "a", mapped: true}, {id: "b"}}, want: 0, wantTok: 2},
		{name: "revoked past the window goes", kind: TokenPurgeRevoked, cutoff: now.Add(-90 * 24 * time.Hour), limit: 10,
			seeds: []purgeSeed{{id: "a", revoked: ago(100, 0), mapped: true}, {id: "b", revoked: ago(100, 0)}}, want: 2, wantTok: 0},
		{name: "boundary: just inside the window stays", kind: TokenPurgeRevoked, cutoff: now.Add(-90 * 24 * time.Hour), limit: 10,
			seeds: []purgeSeed{{id: "a", revoked: ago(90, -time.Hour), mapped: true}, {id: "b", revoked: ago(90, time.Hour), mapped: true}}, want: 1, wantTok: 1},
		{name: "expired path skips revoked and unexpired", kind: TokenPurgeExpired, cutoff: now.Add(-30 * 24 * time.Hour), limit: 10,
			seeds: []purgeSeed{{id: "a", expires: ago(40, 0), mapped: true}, {id: "b", expires: ago(40, 0), revoked: ago(40, 0), mapped: true}, {id: "c", expires: future, mapped: true}, {id: "d", expires: ago(40, 0)}}, want: 2, wantTok: 2},
		{name: "batch limit bounds one call", kind: TokenPurgeRevoked, cutoff: now, limit: 2,
			seeds: []purgeSeed{{id: "a", revoked: ago(5, 0), mapped: true}, {id: "b", revoked: ago(5, 0), mapped: true}, {id: "c", revoked: ago(5, 0), mapped: true}}, want: 2, wantTok: 1},
		{name: "dry run counts without deleting", kind: TokenPurgeRevoked, cutoff: now, limit: 10, dryRun: true,
			seeds: []purgeSeed{{id: "a", revoked: ago(5, 0), mapped: true}}, want: 1, wantTok: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			for _, s := range tc.seeds {
				seedPurgeToken(t, db, s)
			}
			got, err := db.PurgeInactiveAPITokens(context.Background(), tc.kind, tc.cutoff, tc.limit, tc.dryRun)
			if err != nil {
				t.Fatalf("PurgeInactiveAPITokens() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("deleted = %d, want %d", got, tc.want)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM api_tokens`); n != tc.wantTok {
				t.Errorf("mirror rows = %d, want %d", n, tc.wantTok)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM api_token_scopes`); n != tc.wantTok {
				t.Errorf("scope rows = %d, want %d", n, tc.wantTok)
			}
			mapped := 0
			for _, s := range tc.seeds {
				if s.mapped {
					mapped++
				}
			}
			lib := countRows(t, db, `SELECT COUNT(*) FROM theauth_api_tokens`)
			if lib != countRows(t, db, `SELECT COUNT(*) FROM authengine_token_map`) || lib > mapped {
				t.Errorf("library rows %d and id map out of step (mapped seeded %d)", lib, mapped)
			}
		})
	}
}

func TestPurgeInactiveAPITokensRejectsUnknownKind(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.PurgeInactiveAPITokens(context.Background(), TokenPurgeKind("all"), time.Now(), 5, false); err == nil {
		t.Fatal("want an error for an unknown kind")
	}
}
