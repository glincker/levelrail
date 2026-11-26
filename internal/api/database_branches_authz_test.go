package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newBranchPolicyToken builds a read-only token with db:write on app-db and db:admin on app-db-pr-*.
func newBranchPolicyToken(t *testing.T, db *store.DB) string {
	t.Helper()
	const plain = "branch-pr-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_branch_pr", Name: "pr", TokenHash: hashToken(plain), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	attachDoc(t, db, stmtDoc("Allow", `"db:write"`, `"database:app-db"`), store.PrincipalTypeToken, "tok_branch_pr")
	attachDoc(t, db, stmtDoc("Allow", `"db:admin"`, `"database:app-db-pr-*"`), store.PrincipalTypeToken, "tok_branch_pr")
	return plain
}

func branchTokenDo(rt *Router, plain, method, path, body string) int {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(method, path, body, plain))
	return rec.Code
}

func readyBranch(t *testing.T, db *store.DB, name string) {
	t.Helper()
	row, err := db.GetDatabaseBranch(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range [][2]string{{"requested", "cloning"}, {"cloning", "credentials"}, {"credentials", "verifying"}, {"verifying", "ready"}} {
		if err := db.UpdateDatabaseBranchState(context.Background(), row.ID, p[0], p[1], "", now); err != nil {
			t.Fatalf("advance: %v", err)
		}
	}
}

func seedBranchPolicyDatabases(t *testing.T, db *store.DB) {
	t.Helper()
	res := &store.ServiceResources{MemoryBytes: 512 << 20}
	for _, n := range []string{"app-db", "other"} {
		if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: n, Engine: store.EnginePostgres, Version: "16", Resources: res}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBranchAPI_PolicyTokenManagesOwnPRBranches(t *testing.T) {
	rt, db, _, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	plain := newBranchPolicyToken(t, db)
	if code := branchTokenDo(rt, plain, http.MethodPost, "/api/v1/databases/app-db/branches", `{"name":"app-db-pr-7","mode":"schema"}`); code != http.StatusAccepted {
		t.Fatalf("create = %d", code)
	}
	if code := branchTokenDo(rt, plain, http.MethodGet, "/api/v1/databases/app-db/branches/app-db-pr-7", ""); code != http.StatusOK {
		t.Fatalf("show = %d", code)
	}
	readyBranch(t, db, "app-db-pr-7")
	if code := branchTokenDo(rt, plain, http.MethodDelete, "/api/v1/databases/app-db/branches/app-db-pr-7?confirm=app-db-pr-7", ""); code/100 != 2 {
		t.Fatalf("delete = %d", code)
	}
}

func TestBranchAPI_PolicyTokenCannotTouchParentOrOtherBranches(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	plain := newBranchPolicyToken(t, db)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/app-db/branches", `{"name":"app-db-pr-1","mode":"schema"}`)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/other/branches", `{"name":"other-pr-1","mode":"schema"}`)
	readyBranch(t, db, "app-db-pr-1")
	readyBranch(t, db, "other-pr-1")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/databases/app-db", ""},
		{http.MethodDelete, "/api/v1/databases/other/branches/other-pr-1?confirm=other-pr-1", ""},
		{http.MethodGet, "/api/v1/databases/other/branches/other-pr-1", ""},
		{http.MethodPost, "/api/v1/databases/other/branches/other-pr-1/reset", `{"confirm":"other-pr-1"}`},
		{http.MethodDelete, "/api/v1/databases/app-db/branches/other-pr-1?confirm=other-pr-1", ""},
	} {
		if code := branchTokenDo(rt, plain, c.method, c.path, c.body); code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 403 or 404", c.method, c.path, code)
		}
	}
	if _, err := db.GetDatabaseBranch(context.Background(), "other-pr-1"); err != nil {
		t.Fatalf("other branch must survive: %v", err)
	}
}

func TestBranchAPI_FullCopyStillNeedsAdminOnParent(t *testing.T) {
	rt, db, _, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	plain := newBranchPolicyToken(t, db)
	if code := branchTokenDo(rt, plain, http.MethodPost, "/api/v1/databases/app-db/branches", `{"name":"app-db-pr-8","mode":"full"}`); code != http.StatusForbidden {
		t.Fatalf("full copy create = %d, want 403", code)
	}
}

func newBranchTokenWith(t *testing.T, db *store.DB, id string, abilities []string, docs ...string) string {
	t.Helper()
	plain := id + "-secret" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		attachDoc(t, db, d, store.PrincipalTypeToken, id)
	}
	return plain
}

func denied(code int) bool { return code == http.StatusForbidden || code == http.StatusNotFound }

func TestBranchAPI_GlobNameInForeignTreeIsRefused(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	plain := newBranchPolicyToken(t, db)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/other/branches", `{"name":"app-db-pr-9","mode":"schema"}`)
	readyBranch(t, db, "app-db-pr-9")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/databases/other/branches/app-db-pr-9?confirm=app-db-pr-9", ""},
		{http.MethodPost, "/api/v1/databases/other/branches/app-db-pr-9/promote", `{"confirm":"app-db-pr-9"}`},
		{http.MethodPatch, "/api/v1/databases/other/branches/app-db-pr-9", `{"protected":true}`},
		{http.MethodDelete, "/api/v1/databases/app-db/branches/app-db-pr-9?confirm=app-db-pr-9", ""},
	} {
		if code := branchTokenDo(rt, plain, c.method, c.path, c.body); !denied(code) {
			t.Errorf("%s %s = %d, want 403 or 404", c.method, c.path, code)
		}
	}
	if _, err := db.GetDatabaseBranch(context.Background(), "app-db-pr-9"); err != nil {
		t.Fatalf("foreign branch must survive: %v", err)
	}
}

func TestBranchAPI_ExplicitDenyBeatsBranchAdmin(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/app-db/branches", `{"name":"app-db-pr-2","mode":"schema"}`)
	readyBranch(t, db, "app-db-pr-2")
	allow := []string{stmtDoc("Allow", `"db:write"`, `"database:app-db"`), stmtDoc("Allow", `"db:admin"`, `"database:app-db-pr-*"`)}
	cases := []struct {
		name      string
		abilities []string
		extra     string
		method    string
		path      string
		body      string
	}{
		{"deny db:read on branch with admin allow", []string{AbilityRead}, stmtDoc("Deny", `"db:read"`, `"database:app-db-pr-*"`), http.MethodGet, "/api/v1/databases/app-db/branches/app-db-pr-2", ""},
		{"deny db:read on root with admin allow", []string{AbilityRead}, stmtDoc("Deny", `"db:read"`, `"database:app-db"`), http.MethodGet, "/api/v1/databases/app-db/branches/app-db-pr-2/schema-diff?against=app-db", ""},
		{"deny db:write on root with flat root", []string{AbilityRoot}, stmtDoc("Deny", `"db:write"`, `"database:app-db"`), http.MethodPost, "/api/v1/databases/app-db/branches/app-db-pr-2/reset", `{"confirm":"app-db-pr-2"}`},
		{"deny db:write on root with admin allow", []string{AbilityRead}, stmtDoc("Deny", `"db:write"`, `"database:app-db"`), http.MethodDelete, "/api/v1/databases/app-db/branches/app-db-pr-2?confirm=app-db-pr-2", ""},
		{"deny db:read on branch with flat root", []string{AbilityRoot}, stmtDoc("Deny", `"db:read"`, `"database:app-db-pr-2"`), http.MethodGet, "/api/v1/databases/app-db/branches/app-db-pr-2", ""},
	}
	for i, c := range cases {
		plain := newBranchTokenWith(t, db, "tok_deny_"+string(rune('a'+i)), c.abilities, append([]string{c.extra}, allow...)...)
		if code := branchTokenDo(rt, plain, c.method, c.path, c.body); !denied(code) {
			t.Errorf("%s: %s %s = %d, want 403 or 404", c.name, c.method, c.path, code)
		}
	}
	if _, err := db.GetDatabaseBranch(context.Background(), "app-db-pr-2"); err != nil {
		t.Fatalf("branch must survive: %v", err)
	}
}

func TestBranchAPI_FullBranchResetNeedsAdminOnParent(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	seedBranchPolicyDatabases(t, db)
	plain := newBranchPolicyToken(t, db)
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/app-db/branches", `{"name":"app-db-pr-4","mode":"data"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("seed full branch = %d", rec.Code)
	}
	readyBranch(t, db, "app-db-pr-4")
	if code := branchTokenDo(rt, plain, http.MethodPost, "/api/v1/databases/app-db/branches/app-db-pr-4/reset", `{"confirm":"app-db-pr-4"}`); code != http.StatusForbidden {
		t.Fatalf("reset of a full branch = %d, want 403", code)
	}
}

func TestBranchAPI_HiddenRootLooksLikeMissing(t *testing.T) {
	f := newTeamFixture(t)
	cookie := f.cookies[allTeamRoles[0]]
	hiddenCode, hiddenBody := teamDo(t, f.rt, http.MethodGet, "/api/v1/databases/"+dbB+"/branches/zz-missing", "", func(req *http.Request) { req.AddCookie(cookie) })
	missingCode, missingBody := teamDo(t, f.rt, http.MethodGet, "/api/v1/databases/"+dbA+"/branches/zz-missing", "", func(req *http.Request) { req.AddCookie(cookie) })
	if hiddenCode != http.StatusNotFound || missingCode != http.StatusNotFound || !strings.Contains(hiddenBody, `"branch not found"`) || !strings.Contains(missingBody, `"branch not found"`) {
		t.Fatalf("hidden = %d %q, missing = %d %q", hiddenCode, hiddenBody, missingCode, missingBody)
	}
}
