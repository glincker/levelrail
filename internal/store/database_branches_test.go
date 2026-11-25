package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

//nolint:unparam // root is a fixture knob kept for readability
func testBranch(id, name, parent, root string) DatabaseBranch {
	return DatabaseBranch{ID: id, DatabaseName: name, ParentName: parent, RootName: root, Mode: "full", Source: "live", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
}

func seedDatabases(t *testing.T, db *DB, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := db.CreateDesiredDatabase(context.Background(), DesiredDatabase{Name: n, Engine: EnginePostgres, Version: "16"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDatabaseBranchLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "app-a", "app-a-x")
	parent := testBranch("b1", "app-a", "app", "app")
	child := testBranch("b2", "app-a-x", "app-a", "app")
	for _, b := range []DatabaseBranch{parent, child} {
		if err := db.CreateDatabaseBranch(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.GetDatabaseBranch(ctx, "app-a")
	if err != nil || got.State != "requested" || got.ClonePath != "dump" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := db.GetDatabaseBranch(ctx, "nope"); !errors.Is(err, ErrDatabaseBranchNotFound) {
		t.Fatalf("missing = %v", err)
	}
	kids, _ := db.ListDatabaseBranchesByParent(ctx, "app-a")
	root, _ := db.ListDatabaseBranchesByRoot(ctx, "app")
	if len(kids) != 1 || len(root) != 2 {
		t.Fatalf("kids %d root %d", len(kids), len(root))
	}
	if err := db.UpdateDatabaseBranchState(ctx, "b1", "requested", "ready", "", "t"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseBranchState(ctx, "b1", "requested", "cloning", "", "t"); !errors.Is(err, ErrDatabaseBranchStateRace) {
		t.Fatalf("race = %v", err)
	}
	if err := db.DeleteDatabaseBranch(ctx, "app-a"); !errors.Is(err, ErrDatabaseBranchHasChild) {
		t.Fatalf("parent delete = %v", err)
	}
	if err := db.DeleteDesiredDatabase(ctx, "app-a"); !errors.Is(err, ErrDatabaseBranchHasChild) {
		t.Fatalf("parent database delete = %v", err)
	}
	if _, err := db.GetDesiredDatabase(ctx, "app-a"); err != nil {
		t.Fatalf("blocked delete removed the database: %v", err)
	}
	if err := db.DeleteDesiredDatabase(ctx, "ghost"); !errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("ghost delete = %v", err)
	}
	if err := db.DeleteDatabaseBranch(ctx, "app-a-x"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDatabaseBranch(ctx, "app-a"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDatabaseBranch(ctx, "app-a"); !errors.Is(err, ErrDatabaseBranchNotFound) {
		t.Fatalf("gone = %v", err)
	}
}

func TestDatabaseBranchProtectionEnforced(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "app-p")
	b := testBranch("p1", "app-p", "app", "app")
	b.State, b.Protected = "ready", true
	if err := db.CreateDatabaseBranch(ctx, b); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"resetting", "expiring", "deleting"} {
		if err := db.UpdateDatabaseBranchState(ctx, "p1", "ready", to, "", "t"); !errors.Is(err, ErrDatabaseBranchProtected) {
			t.Errorf("%s = %v", to, err)
		}
	}
	if err := db.DeleteDatabaseBranch(ctx, "app-p"); !errors.Is(err, ErrDatabaseBranchProtected) {
		t.Fatalf("delete = %v", err)
	}
	if err := db.DeleteDesiredDatabase(ctx, "app-p"); !errors.Is(err, ErrDatabaseBranchProtected) {
		t.Fatalf("database delete = %v", err)
	}
	if err := db.SetDatabaseBranchProtected(ctx, "p1", false, true, "t"); !errors.Is(err, ErrDatabaseBranchStateRace) {
		t.Fatalf("stale protect = %v", err)
	}
	if err := db.SetDatabaseBranchProtected(ctx, "p1", true, false, "t"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseBranchState(ctx, "p1", "ready", "expiring", "", "t"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseBranchProtected(ctx, "p1", false, true, "t"); !errors.Is(err, ErrDatabaseBranchStateRace) {
		t.Fatalf("protect while leaving = %v", err)
	}
}

func TestDatabaseBranchRowsGoWithDatabase(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "app-x")
	if err := db.CreateDatabaseBranch(ctx, testBranch("x1", "app-x", "app", "app")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveMaskingRuleset(ctx, MaskingRuleset{ID: "r1", DatabaseName: "app-x", CreatedAt: "t"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredDatabase(ctx, "app-x"); err != nil {
		t.Fatal(err)
	}
	seedDatabases(t, db, "app-x")
	if _, err := db.GetDatabaseBranch(ctx, "app-x"); !errors.Is(err, ErrDatabaseBranchNotFound) {
		t.Fatalf("stale branch row = %v", err)
	}
	if _, _, ok, _ := db.GetLatestMaskingRuleset(ctx, "app-x"); ok {
		t.Fatal("stale ruleset")
	}
	if err := db.CreateDatabaseBranch(ctx, testBranch("z", "ghost", "app", "app")); err == nil {
		t.Fatal("branch row without a database must be refused")
	}
}

func TestListExpiredDatabaseBranches(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "due", "later", "never", "locked", "busy", "dead", "stuck", "kidparent", "kid")
	mk := func(id, name, state, exp, upd string, prot bool, parent string) {
		b := testBranch(id, name, parent, "app")
		b.State, b.ExpiresAt, b.Protected = state, exp, prot
		if upd != "" {
			b.UpdatedAt = upd
		}
		if err := db.CreateDatabaseBranch(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	mk("1", "due", "ready", "2026-01-01T02:00:00+02:00", "", false, "app")
	mk("2", "later", "ready", "2026-02-01T00:00:00Z", "", false, "app")
	mk("3", "never", "ready", "", "", false, "app")
	mk("4", "locked", "ready", "2026-01-01T00:00:00Z", "", true, "app")
	mk("5", "busy", "cloning", "2026-01-01T00:00:00Z", "", false, "app")
	mk("6", "dead", "failed", "", "2026-01-01T00:00:00Z", false, "app")
	mk("7", "stuck", "deleting", "", "2026-01-14T00:00:00Z", false, "app")
	mk("8", "kidparent", "ready", "2026-01-01T00:00:00Z", "", false, "app")
	mk("9", "kid", "cloning", "", "", false, "kidparent")
	got, err := db.ListExpiredDatabaseBranches(ctx, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), DuePolicy{FailedRetention: 24 * time.Hour, Retry: time.Hour})
	names := map[string]bool{}
	for _, b := range got {
		names[b.DatabaseName] = true
	}
	if err != nil || len(got) != 3 || !names["due"] || !names["dead"] || !names["stuck"] {
		t.Fatalf("expired = %v, %v", names, err)
	}
	if b, _ := db.GetDatabaseBranch(ctx, "due"); b.ExpiresAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("expiry not normalized: %q", b.ExpiresAt)
	}
	if err := db.CreateDatabaseBranch(ctx, DatabaseBranch{ID: "bad", DatabaseName: "app", ExpiresAt: "soon"}); !errors.Is(err, ErrDatabaseBranchTime) {
		t.Fatalf("bad time = %v", err)
	}
}

func TestCreateDatabaseBranchWithinQuota(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "a", "b", "c", "d")
	q := BranchQuota{MaxPerRoot: 2, MaxTotal: 10, MaxTotalBytes: 1000, ParentBytes: 100}
	for i, n := range []string{"a", "b"} {
		if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch(n, n, "app", "app"), q); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
	}
	if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch("c", "c", "app", "app"), q); !errors.Is(err, ErrDatabaseBranchQuota) {
		t.Fatalf("per root = %v", err)
	}
	// A promoted row no longer counts against the root.
	if err := db.UpdateDatabaseBranchState(ctx, "a", "requested", "cloning", "", "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE database_branches SET state = 'promoted' WHERE id = 'a'`); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch("c", "c", "app", "app"), q); err != nil {
		t.Fatalf("after promote = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE database_branches SET size_bytes = 100`); err != nil {
		t.Fatal(err)
	}
	q.MaxTotalBytes, q.MaxPerRoot = 150, 0
	if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch("d", "d", "app", "app"), q); !errors.Is(err, ErrDatabaseBranchQuota) {
		t.Fatalf("bytes = %v", err)
	}
}

func TestMaskingRulesetVersions(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app")
	if _, _, ok, err := db.GetLatestMaskingRuleset(ctx, "app"); ok || err != nil {
		t.Fatalf("empty = %v, %v", ok, err)
	}
	for i, id := range []string{"r1", "r2"} {
		rs, err := db.SaveMaskingRuleset(ctx, MaskingRuleset{ID: id, DatabaseName: "app", CreatedAt: "t"}, []MaskingRule{{ID: id + "a", Target: "public.users.email", Kind: "hash", Deterministic: true, SaltRef: "k"}})
		if err != nil || rs.Version != i+1 {
			t.Fatalf("save %d = %+v, %v", i, rs, err)
		}
	}
	rs, rules, ok, err := db.GetLatestMaskingRuleset(ctx, "app")
	if err != nil || !ok || rs.Version != 2 || len(rules) != 1 || !rules[0].Deterministic || rules[0].Params != "{}" {
		t.Fatalf("latest = %+v %+v %v %v", rs, rules, ok, err)
	}
	if rs1, _, err := db.GetMaskingRuleset(ctx, "app", 1); err != nil || rs1.ID != "r1" {
		t.Fatalf("v1 = %+v, %v", rs1, err)
	}
	if _, _, err := db.GetMaskingRuleset(ctx, "app", 9); !errors.Is(err, ErrMaskingRulesetNotFound) {
		t.Fatalf("v9 = %v", err)
	}
}

func TestMaskingRuleConstraints(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app")
	bad := []MaskingRule{
		{ID: "a", Target: "", Kind: "null"},
		{ID: "b", Target: "t.c", Kind: "null", Params: "{not json"},
		{ID: "c", Target: "t.c", Kind: "hash", Deterministic: true},
		{ID: "d", Target: "t.c", Kind: "fake", Deterministic: true},
	}
	for i, r := range bad {
		if _, err := db.SaveMaskingRuleset(ctx, MaskingRuleset{ID: "rs" + r.ID, DatabaseName: "app", CreatedAt: "t"}, []MaskingRule{r}); err == nil {
			t.Errorf("rule %d accepted", i)
		}
	}
	if _, err := db.SaveMaskingRuleset(ctx, MaskingRuleset{ID: "ok", DatabaseName: "app", CreatedAt: "t"}, []MaskingRule{{ID: "k", Target: "t.c", Kind: "null"}}); err != nil {
		t.Fatalf("valid rule refused: %v", err)
	}
}

func TestPromoteClearsParentAndExpiry(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "app-p")
	b := testBranch("p", "app-p", "app", "app")
	b.State, b.ExpiresAt = "promoting", "2026-01-01T00:00:00Z"
	if err := db.CreateDatabaseBranch(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseBranchState(ctx, "p", "promoting", "promoted", "", "t"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetDatabaseBranch(ctx, "app-p")
	if got.ParentName != "" || got.ExpiresAt != "" || got.State != "promoted" {
		t.Fatalf("promoted = %+v", got)
	}
}

func TestQuotaClampsNegativeParentBytes(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedDatabases(t, db, "app", "a")
	q := BranchQuota{MaxTotalBytes: 10, ParentBytes: -1 << 40}
	if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch("a", "a", "app", "app"), q); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateDatabaseBranchWithinQuota(ctx, testBranch("a2", "a", "app", "app"), BranchQuota{MaxTotalBytes: 10, ParentBytes: 100}); err == nil {
		t.Fatal("expected refusal")
	}
}

// A later migration that rebuilds desired_databases must also carry the tables that reference it.
func TestLaterMigrationsHandleBranchTables(t *testing.T) {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%d_", &n); err != nil || n <= 3331 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("migrations", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(string(raw))
		rebuilds := strings.Contains(low, "drop table desired_databases") || strings.Contains(low, "alter table desired_databases rename") || strings.Contains(low, "create table desired_databases")
		if rebuilds && !strings.Contains(low, "database_branches") {
			t.Errorf("%s rebuilds desired_databases without handling database_branches", e.Name())
		}
	}
}
