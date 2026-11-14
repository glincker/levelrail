package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStackSyncRoutesAuthz(t *testing.T) {
	rt, db := newTestRouter(t)
	if _, err := db.CreateStack(t.Context(), store.StackRecord{ID: "stk-sync", Name: "infra", Scope: store.StackScopeInstall, DriftPolicy: "warn", CreatedBy: "t"}, time.Now()); err != nil {
		t.Fatalf("create stack: %v", err)
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.test", []string{AbilityRead})
	readCookie := sessionCookieForTest(t, rt, reader.ID)
	admin := storeUserWithAbilitiesForTest(t, db, "root@example.test", []string{AbilityRoot})
	rootCookie := sessionCookieForTest(t, rt, admin.ID)
	call := func(c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, c, method, path, body))
		return rec
	}
	cfg := `{"repo_url":"https://git.example.invalid/org/infra.git","branch":"main","path":"stacks","enabled":false,"apply":false,"prune":false,"deploy":false}`
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/stacks/infra/sync", cfg},
		{http.MethodPost, "/api/v1/stacks/infra/sync/refresh", ""},
	} {
		if rec := call(readCookie, c.method, c.path, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("reader %s %s = %d, want 403", c.method, c.path, rec.Code)
		}
	}
	if rec := call(rootCookie, http.MethodPut, "/api/v1/stacks/infra/sync", strings.Replace(cfg, `"stacks"`, `"../etc"`, 1)); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "sync_path_invalid") {
		t.Fatalf("traversal path = %d %s, want 422 sync_path_invalid", rec.Code, rec.Body.String())
	}
	if rec := call(rootCookie, http.MethodPut, "/api/v1/stacks/infra/sync", strings.Replace(cfg, "https://git.example.invalid/org/infra.git", "file:///etc", 1)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("file url = %d, want 422", rec.Code)
	}
	if rec := call(rootCookie, http.MethodPut, "/api/v1/stacks/infra/sync", cfg); rec.Code != http.StatusOK {
		t.Fatalf("root put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(readCookie, http.MethodGet, "/api/v1/stacks/infra/sync", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"refresh_cooldown_seconds":`) {
		t.Fatalf("reader get = %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(rootCookie, http.MethodPost, "/api/v1/stacks/infra/sync/refresh", ""); rec.Code != http.StatusConflict {
		t.Fatalf("refresh while disabled = %d, want 409", rec.Code)
	}
}
