package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeBranchCloner struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeBranchCloner) Run(_ context.Context, b store.DatabaseBranch, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, b.DatabaseName)
	return nil
}

func newLiveBranchRouter(t *testing.T) (*Router, *store.DB, *http.Cookie, *fakeBranchCloner) {
	t.Helper()
	cl := &fakeBranchCloner{}
	rt, db, _ := newBranchRouter(t, nil)
	rt.branchCloner = cl
	cookie := loginTestSession(t, rt, db)
	res := &store.ServiceResources{MemoryBytes: 512 << 20}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", Resources: res}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return rt, db, cookie, cl
}

func branchDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestBranchAPI_CreateListShowAndClone(t *testing.T) {
	rt, db, cookie, cl := newLiveBranchRouter(t)
	rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"main-try","mode":"data","ttl":"7d"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	row, err := db.GetDatabaseBranch(context.Background(), "main-try")
	if err != nil || row.ParentName != "main" || row.RootName != "main" || row.Mode != "full" || row.ExpiresAt == "" {
		t.Fatalf("row = %+v err=%v", row, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cl.mu.Lock()
		n := len(cl.calls)
		cl.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(cl.calls) != 1 {
		t.Fatalf("cloner calls = %v", cl.calls)
	}
	if rec := branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/branches", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"main-try"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/branches/main-try", ""); rec.Code != http.StatusOK {
		t.Fatalf("show = %d", rec.Code)
	}
	if rec := branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/branches/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("show missing = %d", rec.Code)
	}
}

func TestBranchAPI_ProtectedDeleteAndParentDeleteRefused(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","mode":"schema"}`)
	if rec := branchDo(t, rt, cookie, http.MethodPatch, "/api/v1/databases/main/branches/b1", `{"protected":true}`); rec.Code != http.StatusOK {
		t.Fatalf("protect = %d %s", rec.Code, rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/branches/b1?confirm=wrong", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d", rec.Code)
	}
	if rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/branches/b1?confirm=b1", ""); rec.Code != http.StatusConflict {
		t.Fatalf("protected delete = %d %s", rec.Code, rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main", ""); rec.Code != http.StatusConflict {
		t.Fatalf("parent delete with child = %d", rec.Code)
	}
	if _, err := db.GetDatabaseBranch(context.Background(), "b1"); err != nil {
		t.Fatalf("protected branch must survive: %v", err)
	}
}

func TestBranchAPI_QuotaRefusesSecondBranch(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) { return "1", k == "APP_BRANCHES_MAX_PER_ROOT" })
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("first = %d %s", rec.Code, rec.Body.String())
	}
	rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b2"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "quota") {
		t.Fatalf("second = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "b2"); err == nil {
		t.Fatal("a refused branch must not leave a database behind")
	}
}

func TestBranchAPI_DiskRefusalIs409WithReason(t *testing.T) {
	rt, _, _, _ := newLiveBranchRouter(t)
	rec := httptest.NewRecorder()
	if !rt.writeBranchRefusal(rec, &diskguard.LowDiskError{Op: diskguard.OpBranch, Volume: "data"}) || rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodeLowDisk) {
		t.Fatalf("refusal = %d %s", rec.Code, rec.Body.String())
	}
}

func TestBranchAPI_PromoteDetachesAndResetNeedsReady(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1"}`)
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches/b1/promote", `{"confirm":"b1"}`); rec.Code != http.StatusConflict {
		t.Fatalf("promote of a branch that is not ready = %d", rec.Code)
	}
	row, _ := db.GetDatabaseBranch(context.Background(), "b1")
	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range [][2]string{{"requested", "cloning"}, {"cloning", "credentials"}, {"credentials", "verifying"}, {"verifying", "ready"}} {
		if err := db.UpdateDatabaseBranchState(context.Background(), row.ID, p[0], p[1], "", now); err != nil {
			t.Fatalf("advance: %v", err)
		}
	}
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches/b1/promote", `{"confirm":"b1"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetDatabaseBranch(context.Background(), "b1")
	if got.State != "promoted" || got.ParentName != "" {
		t.Fatalf("promoted row = %+v", got)
	}
}

func TestBranchAPI_AuthzDenyReadOnlyToken(t *testing.T) {
	rt, db, _, _ := newLiveBranchRouter(t)
	const plain = "branch-read-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_branch_ro", Name: "ro", TokenHash: hashToken(plain), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/databases/main/branches", `{"name":"x"}`},
		{http.MethodGet, "/api/v1/databases/main/branches", ""},
		{http.MethodDelete, "/api/v1/databases/main/branches/x?confirm=x", ""},
		{http.MethodPatch, "/api/v1/databases/main/branches/x", `{"protected":true}`},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, bearerRequest(c.method, c.path, c.body, plain))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, rec.Code)
		}
	}
}

func TestBranchAPI_InFlightBranchIsBusyAndUnreachable(t *testing.T) {
	rt, _, cookie, _ := newLiveBranchRouter(t)
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1"}`)
	rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/branches/b1?confirm=b1", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete while copying = %d %s", rec.Code, rec.Body.String())
	}
	rec = branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/b1/schema", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "branch_not_ready") {
		t.Fatalf("explorer on a copying branch = %d %s", rec.Code, rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/branches/b1", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete without confirm = %d", rec.Code)
	}
}

func TestBranchAPI_MinTTLAndTypedConfirm(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","ttl":"1s"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("1s ttl = %d %s", rec.Code, rec.Body.String())
	}
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b2"}`)
	for _, sub := range []string{"reset", "promote"} {
		if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches/b2/"+sub, `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s with wrong confirm = %d", sub, rec.Code)
		}
	}
}
