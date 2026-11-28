package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

func tokenCall(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v (%s)", target, err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestTokenListFiltersGroupsAndKeepsOldShape(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for i := 0; i < 3; i++ {
		mintTokenAs(t, rt, cookie, "ci")
	}
	mintTokenAs(t, rt, cookie, "deploy")

	var legacy []tokenResource
	if code := tokenCall(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens", "", &legacy); code != http.StatusOK || len(legacy) < 4 {
		t.Fatalf("old shape: code %d, %d items", code, len(legacy))
	}
	for _, tok := range legacy {
		if tok.Status != tokenStatusActive {
			t.Errorf("status = %q, want active", tok.Status)
		}
	}
	var page tokenListPage
	if code := tokenCall(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens?name=ci&limit=2&sort=name", "", &page); code != http.StatusOK {
		t.Fatalf("code %d", code)
	}
	if page.Total != 3 || len(page.Items) != 2 || page.NextCursor != "2" || page.CountsByStatus[tokenStatusActive] != 3 {
		t.Fatalf("page = %+v", page)
	}
	var groups tokenGroupsPage
	tokenCall(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens?groups=name&q=ci", "", &groups)
	if len(groups.Groups) != 1 || groups.Groups[0].Name != "ci" || groups.Groups[0].Count != 3 || groups.Groups[0].Active != 3 {
		t.Fatalf("groups = %+v", groups)
	}
	if code := tokenCall(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens?status=bogus", "", nil); code != http.StatusBadRequest {
		t.Errorf("bad status code = %d, want 400", code)
	}
}

func TestBulkRevokeSafety(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) {
		if k == "APP_TOKENS_BULK_MAX" {
			return "3", true
		}
		return "", false
	})
	cookie := loginTestSession(t, rt, db)
	for i := 0; i < 3; i++ {
		mintTokenAs(t, rt, cookie, "ci")
	}
	keep := mintTokenAs(t, rt, cookie, "keep")
	const path = "/api/v1/tokens/bulk-revoke"

	var dry bulkRevokeResponse
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"filter":{"name":"ci"},"dry_run":true}`, &dry); code != http.StatusOK || dry.Count != 3 || len(dry.Sample) != 3 {
		t.Fatalf("dry run: code %d, %+v", code, dry)
	}
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"filter":{"name":"ci"},"confirm_count":2}`, nil); code != http.StatusConflict {
		t.Errorf("mismatched confirm_count code = %d, want 409", code)
	}
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"filter":{"name":"ci"}}`, nil); code != http.StatusConflict {
		t.Errorf("missing confirm_count code = %d, want 409", code)
	}
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"filter":{"status":"active"},"confirm_count":4}`, nil); code != http.StatusBadRequest {
		t.Errorf("over tokens.bulk_max code = %d, want 400", code)
	}
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"ids":["a"],"filter":{}}`, nil); code != http.StatusBadRequest {
		t.Errorf("ids and filter together code = %d, want 400", code)
	}
	var done bulkRevokeResponse
	if code := tokenCall(t, rt, cookie, http.MethodPost, path, `{"filter":{"name":"ci"},"confirm_count":3}`, &done); code != http.StatusOK || done.Revoked != 3 {
		t.Fatalf("revoke: code %d, %+v", code, done)
	}
	if !tokenUsable(rt, keep.Token) {
		t.Error("a token outside the filter must stay valid")
	}
	var again bulkRevokeResponse
	tokenCall(t, rt, cookie, http.MethodPost, path, fmt.Sprintf(`{"ids":[%q]}`, keep.ID), &again)
	if again.Revoked != 1 || tokenUsable(rt, keep.Token) {
		t.Errorf("revoke by id = %+v", again)
	}
	entries, err := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, e := range entries {
		if e.Action == auditActionTokensBulkRevoke {
			rows++
			if strings.Contains(e.Detail, keep.Token) {
				t.Error("audit detail must never hold a secret")
			}
		}
	}
	if rows != 2 {
		t.Errorf("bulk revoke audit rows = %d, want one per real run (2)", rows)
	}
}

func TestPruneNeverDeletesValidTokens(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	live := mintTokenAs(t, rt, cookie, "live")
	var out tokenPruneResponse
	if code := tokenCall(t, rt, cookie, http.MethodPost, "/api/v1/tokens/prune", `{"older_than_days":0}`, &out); code != http.StatusOK {
		t.Fatalf("prune code = %d", code)
	}
	if out.Revoked != 0 || out.Expired != 0 || !tokenUsable(rt, live.Token) {
		t.Errorf("prune removed a valid token: %+v", out)
	}
	if code := tokenCall(t, rt, cookie, http.MethodPost, "/api/v1/tokens/prune", `{}`, nil); code != http.StatusBadRequest {
		t.Errorf("missing older_than_days code = %d, want 400", code)
	}
}
