package api

import (
	"context"
	"net/http"
	"net/http/httptest"
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
