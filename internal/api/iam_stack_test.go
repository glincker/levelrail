package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// stackTokenFixture seeds stack shop-prod (project p-shop) and blog-prod
// (project p-blog), and mints a shop-prod token with the given verbs.
func stackTokenFixture(t *testing.T, verbs []string, abilities []string) (*Router, *store.DB, *http.Cookie, string, string) {
	t.Helper()
	rt, db, cookie := stackFixture(t)
	if err := db.SaveProject(t.Context(), store.Project{ID: "p-blog", Name: "blog", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	body := jsonBody(t, map[string]any{"name": "blog-prod", "project": "blog"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", body), nil); code != http.StatusCreated {
		t.Fatalf("create blog stack status = %d", code)
	}
	tok, id := mintStackToken(t, rt, cookie, "shop-prod", verbs, abilities)
	return rt, db, cookie, tok, id
}

func mintStackToken(t *testing.T, rt *Router, cookie *http.Cookie, stack string, verbs, abilities []string) (string, string) {
	t.Helper()
	var out createTokenResponse
	body := jsonBody(t, map[string]any{"name": "ci:" + stack, "abilities": abilities, "stack": stack, "stack_verbs": verbs})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body), &out); code != http.StatusCreated {
		t.Fatalf("mint stack token status = %d", code)
	}
	return out.Token, out.ID
}

func callWithToken(t *testing.T, rt *Router, tok, method, path, body string) int {
	t.Helper()
	return iacCall(t, rt, bearerRequest(method, path, body, tok), nil)
}

var applyVerbs = []string{store.StackVerbPlan, store.StackVerbApply}

func TestStackTokenPlansAndAppliesOwnStack(t *testing.T) {
	rt, db, _, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})

	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusOK {
		t.Fatalf("plan status = %d", code)
	}
	var out stackApplyResponse
	code := iacCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, nil), tok), &out)
	if code != http.StatusOK || out.Revision == nil || out.Revision.Status != store.StackRevApplied {
		t.Fatalf("apply: status %d %+v", code, out)
	}
	if svc, err := db.GetDesiredService(t.Context(), "web"); err != nil || svc.ProjectID != "p-shop" {
		t.Fatalf("app = %+v, %v", svc, err)
	}
	for _, p := range []string{"/api/v1/stacks/shop-prod", "/api/v1/stacks/shop-prod/revisions", "/api/v1/stacks/shop-prod/revisions/1", "/api/v1/stacks/shop-prod/revisions/1/diff"} {
		if code := callWithToken(t, rt, tok, http.MethodGet, p, ""); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, code)
		}
	}
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""); code != http.StatusOK {
		t.Errorf("rollback = %d, want 200", code)
	}
}

func TestStackTokenCannotTouchAnotherStack(t *testing.T) {
	rt, _, _, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/stacks/blog-prod/plan"},
		{http.MethodPost, "/api/v1/stacks/blog-prod/apply"},
		{http.MethodGet, "/api/v1/stacks/blog-prod"},
		{http.MethodGet, "/api/v1/stacks/blog-prod/revisions"},
		{http.MethodPost, "/api/v1/stacks/blog-prod/revisions/1/rollback"},
		{http.MethodGet, "/api/v1/stacks"},
		{http.MethodPost, "/api/v1/stacks"},
	} {
		if code := callWithToken(t, rt, tok, c.method, c.path, stackBody(t, iacTestDocs, nil)); code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, code)
		}
	}
}

func TestStackTokenDeniedOnDirectResourceAndControlPlaneRoutes(t *testing.T) {
	rt, _, _, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/apps", ""},
		{http.MethodPost, "/api/v1/apps", `{"name":"x","image":"nginx","project_id":"p-shop"}`},
		{http.MethodGet, "/api/v1/apps/web", ""},
		{http.MethodPut, "/api/v1/apps/web", `{}`},
		{http.MethodGet, "/api/v1/databases", ""},
		{http.MethodPost, "/api/v1/databases", `{"name":"db","engine":"postgres"}`},
		{http.MethodGet, "/api/v1/projects", ""},
		{http.MethodPost, "/api/v1/apply", stackBody(t, iacTestDocs, nil)},
		{http.MethodPost, "/api/v1/apply/plan", stackBody(t, iacTestDocs, nil)},
		{http.MethodGet, "/api/v1/audit-log", ""},
		{http.MethodGet, "/api/v1/settings/oauth", ""},
		{http.MethodGet, "/api/v1/apps/web/secrets", ""},
		{http.MethodPut, "/api/v1/apps/web/secrets/KEY", `{"value":"x"}`},
	} {
		if code := callWithToken(t, rt, tok, c.method, c.path, c.body); code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, code)
		}
	}
	// Token management is session only, so a stack token never reaches it.
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/auth/tokens", `{"name":"x","abilities":["root"]}`); code < 400 {
		t.Errorf("token mint by stack token = %d", code)
	}
	if code := callWithToken(t, rt, tok, http.MethodGet, "/api/v1/auth/tokens", ""); code < 400 {
		t.Errorf("token list by stack token = %d", code)
	}
}

func TestStackTokenCannotDestroyUnlockPinOrChangeSettings(t *testing.T) {
	rt, _, _, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	for _, c := range []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/stacks/shop-prod?mode=destroy", ""},
		{http.MethodDelete, "/api/v1/stacks/shop-prod", ""},
		{http.MethodPost, "/api/v1/stacks/shop-prod/unlock", ""},
		{http.MethodPatch, "/api/v1/stacks/shop-prod", `{"allow_prune":true}`},
		{http.MethodPut, "/api/v1/stacks/shop-prod/pin", `{"revision":1}`},
		{http.MethodDelete, "/api/v1/stacks/shop-prod/pin", ""},
	} {
		if code := callWithToken(t, rt, tok, c.method, c.path, c.body); code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, code)
		}
	}
}

func TestStackTokenPruneOnlyWithServerAllowPrune(t *testing.T) {
	rt, _, cookie, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, map[string]any{"prune": true})); code != http.StatusForbidden {
		t.Fatalf("prune without allow_prune = %d, want 403", code)
	}
	for _, extra := range []map[string]any{{"adopt": true}, {"allow_mass_prune": true}, {"Adopt": true}} {
		if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, extra)); code != http.StatusForbidden {
			t.Errorf("apply %v = %d, want 403", extra, code)
		}
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", `{"allow_prune":true}`), nil); code != http.StatusOK {
		t.Fatalf("admin allow_prune = %d", code)
	}
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, map[string]any{"prune": true})); code != http.StatusOK {
		t.Fatalf("prune with allow_prune = %d, want 200", code)
	}
}

func TestStackTokenPlanOnlyCannotApply(t *testing.T) {
	rt, _, _, tok, _ := stackTokenFixture(t, []string{store.StackVerbPlan}, []string{AbilityRead})
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusOK {
		t.Fatalf("plan = %d", code)
	}
	for _, p := range []string{"/api/v1/stacks/shop-prod/apply", "/api/v1/stacks/shop-prod/revisions/1/rollback"} {
		if code := callWithToken(t, rt, tok, http.MethodPost, p, stackBody(t, iacTestDocs, nil)); code != http.StatusForbidden {
			t.Errorf("POST %s = %d, want 403", p, code)
		}
	}
}

func TestStackTokenExpiredOrRevokedStaysRejected(t *testing.T) {
	rt, db, cookie, tok, id := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/auth/tokens/"+id, ""), nil); code >= 300 {
		t.Fatalf("revoke = %d", code)
	}
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusUnauthorized {
		t.Errorf("revoked plan = %d, want 401", code)
	}

	const plaintext = "expired-stack-token" //nolint:gosec // test fixture
	past := time.Now().Add(-time.Hour)
	if err := db.SaveAPIToken(t.Context(), store.APIToken{ID: "tok_exp_stack", Name: "ci", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	stack, _ := db.GetStackByName(t.Context(), "shop-prod")
	scope := store.APITokenStackScope{TokenID: "tok_exp_stack", StackID: stack.ID, Verbs: []string{store.StackVerbPlan}} //nolint:gosec // test fixture id, not a credential
	if err := db.SetAPITokenStackScope(t.Context(), scope); err != nil {
		t.Fatal(err)
	}
	if code := callWithToken(t, rt, plaintext, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusUnauthorized {
		t.Errorf("expired plan = %d, want 401", code)
	}
}

func TestStackTokenDeniedOnceItsStackIsGone(t *testing.T) {
	rt, _, cookie, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/shop-prod?mode=forget", ""), nil); code >= 300 {
		t.Fatalf("forget stack = %d", code)
	}
	body := jsonBody(t, map[string]any{"name": "shop-prod", "project": "shop"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", body), nil); code != http.StatusCreated {
		t.Fatalf("recreate stack = %d", code)
	}
	// A new stack with the same name is a different stack.
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusForbidden {
		t.Errorf("plan on recreated stack = %d, want 403", code)
	}
}

func TestLegacyTokenUnchangedByStackScopes(t *testing.T) {
	rt, _, cookie, _, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	var out createTokenResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"legacy","abilities":["read","write"]}`), &out); code != http.StatusCreated {
		t.Fatalf("mint legacy = %d", code)
	}
	if out.Scope != nil {
		t.Errorf("legacy token has a scope: %+v", out.Scope)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/apps"},
		{http.MethodGet, "/api/v1/stacks"},
		{http.MethodGet, "/api/v1/stacks/blog-prod"},
		{http.MethodPost, "/api/v1/stacks/blog-prod/plan"},
	} {
		if code := callWithToken(t, rt, out.Token, c.method, c.path, stackBody(t, iacTestDocs, nil)); code != http.StatusOK {
			t.Errorf("legacy %s %s = %d, want 200", c.method, c.path, code)
		}
	}
}

func TestStackTokenMintValidation(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"name":"a","abilities":["read"],"stack":"shop-prod","stack_verbs":["destroy"]}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["read"],"stack":"shop-prod","stack_verbs":["unlock"]}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["root"],"stack":"shop-prod","stack_verbs":["apply"]}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["read","write"],"stack":"shop-prod"}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["read","write_sensitive"],"stack":"shop-prod","stack_verbs":["apply"]}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["read"],"stack":"nope","stack_verbs":["plan"]}`, http.StatusBadRequest},
		{`{"name":"a","abilities":["read"],"stack_verbs":["plan"]}`, http.StatusBadRequest},
	} {
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", c.body), nil); code != c.want {
			t.Errorf("mint %s = %d, want %d", c.body, code, c.want)
		}
	}
	var out createTokenResponse
	body := `{"name":"ci","abilities":["read"],"stack":"shop-prod","stack_verbs":["plan"]}`
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body), &out); code != http.StatusCreated {
		t.Fatalf("mint = %d", code)
	}
	if out.Scope == nil || out.Scope.StackID == "" || len(out.Scope.StackVerbs) != 1 {
		t.Errorf("scope not reported: %+v", out.Scope)
	}
}

// subRequest builds what a loopback sub-request looks like after mux routing.
func subRequest(ctx context.Context, tok, pattern, body string, values map[string]string) *http.Request {
	method, path, _ := strings.Cut(pattern, " ")
	for k, v := range values {
		path = strings.Replace(path, "{"+k+"}", v, 1)
	}
	req := bearerRequest(method, path, body, tok).WithContext(ctx)
	req.Pattern = pattern
	for k, v := range values {
		req.SetPathValue(k, v)
	}
	return req
}

func TestStackTokenSubRequestsBoundedToLedgerAndProject(t *testing.T) {
	rt, db, _, tok, id := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	ctx := t.Context()
	shop, _ := db.GetStackByName(ctx, "shop-prod")
	blog, _ := db.GetStackByName(ctx, "blog-prod")
	for _, a := range []struct{ name, project string }{{"web", "p-shop"}, {"loose", "p-shop"}, {"intruder", "p-blog"}, {"blogapp", "p-blog"}} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: a.name, Image: "nginx", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceProject(ctx, a.name, a.project); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "orphan", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []store.StackLedgerEntry{{StackID: shop.ID, Kind: "App", Key: "App/web"}, {StackID: blog.ID, Kind: "App", Key: "App/blogapp"}} {
		if err := db.UpsertStackResource(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	call := &stackTokenCall{tokenID: id, stackID: shop.ID, created: map[string]bool{}}
	planCtx := context.WithValue(ctx, stackTokenCallKey{}, call)
	applyCtx := iac.WithStackScope(planCtx, iac.StackScope{StackID: shop.ID, StackName: shop.Name, Revision: 1})
	otherApply := iac.WithStackScope(planCtx, iac.StackScope{StackID: blog.ID, StackName: blog.Name, Revision: 1})
	wrongToken := iac.WithStackScope(context.WithValue(ctx, stackTokenCallKey{}, &stackTokenCall{tokenID: "someone-else", stackID: shop.ID, created: map[string]bool{}}), iac.StackScope{StackID: shop.ID, Revision: 1})

	cases := []struct {
		name    string
		ctx     context.Context
		pattern string
		body    string
		values  map[string]string
		allow   bool
	}{
		{"read own app in plan", planCtx, "GET /api/v1/apps/{name}", "", map[string]string{"name": "web"}, true},
		{"read list in plan", planCtx, "GET /api/v1/apps", "", nil, true},
		{"read missing app", planCtx, "GET /api/v1/apps/{name}", "", map[string]string{"name": "new"}, true},
		{"read other project app", planCtx, "GET /api/v1/apps/{name}", "", map[string]string{"name": "intruder"}, false},
		{"read other stack app", planCtx, "GET /api/v1/apps/{name}", "", map[string]string{"name": "blogapp"}, false},
		{"read secrets names", planCtx, "GET /api/v1/apps/{name}/secrets", "", map[string]string{"name": "web"}, false},
		{"read other project env", planCtx, "GET /api/v1/projects/{id}/env", "", map[string]string{"id": "p-blog"}, false},
		{"write during plan", planCtx, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "web"}, false},
		{"write own app", applyCtx, "PUT /api/v1/apps/{name}", `{"project_id":"p-shop"}`, map[string]string{"name": "web"}, true},
		{"write unowned app in project", applyCtx, "PUT /api/v1/apps/{name}/loadbalancer", `{}`, map[string]string{"name": "loose"}, true},
		{"write app in other project", applyCtx, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "intruder"}, false},
		{"write app owned by other stack", applyCtx, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "blogapp"}, false},
		{"write missing app", applyCtx, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "ghost"}, false},
		{"move own app out", applyCtx, "PUT /api/v1/apps/{name}/project", `{"project_id":"p-blog"}`, map[string]string{"name": "web"}, false},
		{"move via folded key", applyCtx, "PUT /api/v1/apps/{name}", `{"Project_ID":"p-blog"}`, map[string]string{"name": "web"}, false},
		{"duplicate folded keys", applyCtx, "PUT /api/v1/apps/{name}", `{"project_id":"p-shop","PROJECT_ID":"p-blog"}`, map[string]string{"name": "web"}, false},
		{"non-string project", applyCtx, "PUT /api/v1/apps/{name}", `{"project_id":7}`, map[string]string{"name": "web"}, false},
		{"set secret", applyCtx, "PUT /api/v1/apps/{name}/secrets/{key}", `{"value":"x"}`, map[string]string{"name": "web", "key": "K"}, false},
		{"exec", applyCtx, "POST /api/v1/apps/{name}/exec", `{}`, map[string]string{"name": "web"}, false},
		{"reveal env", applyCtx, "POST /api/v1/apps/{name}/env/reveal", `{}`, map[string]string{"name": "web"}, false},
		{"create app in project", applyCtx, "POST /api/v1/apps", `{"name":"fresh","project_id":"p-shop"}`, nil, true},
		{"create app elsewhere", applyCtx, "POST /api/v1/apps", `{"name":"fresh","project_id":"p-blog"}`, nil, false},
		{"create app no project", applyCtx, "POST /api/v1/apps", `{"name":"fresh"}`, nil, false},
		{"create app owned by other stack", applyCtx, "POST /api/v1/apps", `{"name":"blogapp","project_id":"p-shop"}`, nil, false},
		{"claim projectless db", applyCtx, "PUT /api/v1/databases/{name}/project", `{"project_id":"p-shop"}`, map[string]string{"name": "orphan"}, false},
		{"create project", applyCtx, "POST /api/v1/projects", `{"name":"evil"}`, nil, false},
		{"other project env", applyCtx, "PUT /api/v1/projects/{id}/env", `{}`, map[string]string{"id": "p-blog"}, false},
		{"own project env", applyCtx, "PUT /api/v1/projects/{id}/env", `{}`, map[string]string{"id": "p-shop"}, true},
		{"create pipeline on own app", applyCtx, "POST /api/v1/apps/{name}/pipelines", `{"name":"p","yaml":"x"}`, map[string]string{"name": "web"}, false},
		{"update pipeline on own app", applyCtx, "PUT /api/v1/apps/{name}/pipelines/{pname}", `{"yaml":"x"}`, map[string]string{"name": "web", "pname": "p"}, false},
		{"delete pipeline on own app", applyCtx, "DELETE /api/v1/apps/{name}/pipelines/{pname}", "", map[string]string{"name": "web", "pname": "p"}, true},
		{"create alert on own app", applyCtx, "POST /api/v1/apps/{name}/alerts", `{"name":"a"}`, map[string]string{"name": "web"}, true},
		{"delete database", applyCtx, "DELETE /api/v1/databases/{name}", "", map[string]string{"name": "orphan"}, false},
		{"prune without allow_prune", applyCtx, "DELETE /api/v1/apps/{name}", "", map[string]string{"name": "web"}, false},
		{"other stack apply scope", otherApply, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "web"}, false},
		{"marker for another token", wrongToken, "PUT /api/v1/apps/{name}", `{}`, map[string]string{"name": "web"}, false},
		{"stack route inside apply", applyCtx, "POST /api/v1/stacks/{name}/apply", `{}`, map[string]string{"name": "blog-prod"}, false},
		{"settings inside apply", applyCtx, "PATCH /api/v1/stacks/{name}", `{"allow_prune":true}`, map[string]string{"name": "shop-prod"}, false},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		_, ok := rt.gateStackToken(rec, subRequest(c.ctx, tok, c.pattern, c.body, c.values), id)
		if ok != c.allow {
			t.Errorf("%s: allowed = %v, want %v (status %d)", c.name, ok, c.allow, rec.Code)
		}
	}

	// A database created in this apply may then be placed in the stack project.
	rec := httptest.NewRecorder()
	if _, ok := rt.gateStackToken(rec, subRequest(applyCtx, tok, "POST /api/v1/databases", `{"name":"newdb"}`, nil), id); !ok {
		t.Fatalf("create db denied: %d", rec.Code)
	}
	if _, ok := rt.gateStackToken(httptest.NewRecorder(), subRequest(applyCtx, tok, "PUT /api/v1/databases/{name}/project", `{"project_id":"p-shop"}`, map[string]string{"name": "newdb"}), id); !ok {
		t.Error("project for created db denied")
	}

	// Prune: allowed only with allow_prune, and only for what the ledger owns.
	if err := db.UpdateStackSettings(ctx, shop.ID, true, shop.DriftPolicy, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.gateStackToken(httptest.NewRecorder(), subRequest(applyCtx, tok, "DELETE /api/v1/apps/{name}", "", map[string]string{"name": "web"}), id); !ok {
		t.Error("prune of owned app denied with allow_prune")
	}
	if _, ok := rt.gateStackToken(httptest.NewRecorder(), subRequest(applyCtx, tok, "DELETE /api/v1/apps/{name}", "", map[string]string{"name": "loose"}), id); ok {
		t.Error("prune of unowned app allowed")
	}
}

func TestStackTokenBodyPeekKeepsBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"a":1}`))
	raw, r2, err := peekBody(req)
	if err != nil || string(raw) != `{"a":1}` {
		t.Fatalf("peek = %q, %v", raw, err)
	}
	var v map[string]int
	if err := json.NewDecoder(r2.Body).Decode(&v); err != nil || v["a"] != 1 {
		t.Fatalf("body after peek = %v, %v", v, err)
	}
}

func TestStackTokenPlanDoesNotRevealOtherProjectsApps(t *testing.T) {
	rt, db, _, tok, _ := stackTokenFixture(t, []string{store.StackVerbPlan}, []string{AbilityRead})
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "taken", Image: "nginx:1", Port: 80, ProjectID: "p-blog"}); err != nil {
		t.Fatal(err)
	}
	planApp := func(name string) (int, string) {
		docs := strings.ReplaceAll(iacTestDocs, "{name: web}", "{name: "+name+"}")
		var out stackPlanResponse
		code := iacCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, docs, nil), tok), &out)
		for _, c := range out.Plan.Changes {
			if c.Kind == iac.KindApp {
				return code, string(c.Action) + "|" + c.Reason
			}
		}
		return code, "missing"
	}
	foreignCode, foreign := planApp("taken")
	ghostCode, ghost := planApp("ghost")
	if foreignCode != ghostCode || foreign != ghost {
		t.Fatalf("existing foreign app %d %q differs from missing app %d %q", foreignCode, foreign, ghostCode, ghost)
	}
}

func TestStackTokenReappliesOwnAppAndProtectionStillRefuses(t *testing.T) {
	rt, db, cookie, tok, _ := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	apply := func(docs string) (int, stackApplyResponse, string) {
		var out stackApplyResponse
		code, raw := rawCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, docs, nil), tok))
		_ = json.Unmarshal([]byte(raw), &out)
		return code, out, raw
	}
	if code, out, raw := apply(iacTestDocs); code != http.StatusOK || out.Revision == nil || out.Revision.Status != store.StackRevApplied {
		t.Fatalf("first apply = %d %s", code, raw)
	}
	v2 := strings.Replace(iacTestDocs, "ghcr.io/acme/web:1.2.3", "ghcr.io/acme/web:1.2.4", 1)
	code, out, raw := apply(v2)
	if code != http.StatusOK || out.Revision == nil || out.Revision.Status != store.StackRevApplied || strings.Contains(raw, iac.CodePermissionDenied) {
		t.Fatalf("second apply of owned app = %d %s", code, raw)
	}
	if svc, err := db.GetDesiredService(t.Context(), "web"); err != nil || svc.Image != "ghcr.io/acme/web:1.2.4" {
		t.Fatalf("app after second apply = %+v, %v", svc, err)
	}

	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/protection", `{"reason":"freeze"}`), nil); code != http.StatusOK {
		t.Fatalf("protect = %d", code)
	}
	v3 := strings.Replace(iacTestDocs, "ghcr.io/acme/web:1.2.3", "ghcr.io/acme/web:1.2.5", 1)
	if code, out, raw := apply(v3); code == http.StatusOK && out.Revision != nil && out.Revision.Status == store.StackRevApplied {
		t.Fatalf("apply over a protected app succeeded: %s", raw)
	}
	if svc, _ := db.GetDesiredService(t.Context(), "web"); svc.Image != "ghcr.io/acme/web:1.2.4" {
		t.Fatalf("protected app changed to %s", svc.Image)
	}
}

func TestStackTokenProtectionReadOnlyForOwnedApps(t *testing.T) {
	rt, db, _, tok, id := stackTokenFixture(t, applyVerbs, []string{AbilityRead, AbilityWrite})
	ctx := t.Context()
	shop, _ := db.GetStackByName(ctx, "shop-prod")
	blog, _ := db.GetStackByName(ctx, "blog-prod")
	for _, a := range []struct{ name, project string }{{"web", "p-shop"}, {"loose", "p-shop"}, {"blogapp", "p-blog"}, {"shopblog", "p-shop"}} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: a.name, Image: "nginx", Port: 80, ProjectID: a.project}); err != nil {
			t.Fatal(err)
		}
	}
	// shopblog sits in the shop project but another stack owns it.
	for _, e := range []store.StackLedgerEntry{{StackID: shop.ID, Kind: "App", Key: "App/web"}, {StackID: blog.ID, Kind: "App", Key: "App/blogapp"}, {StackID: blog.ID, Kind: "App", Key: "App/shopblog"}} {
		if err := db.UpsertStackResource(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	call := &stackTokenCall{tokenID: id, stackID: shop.ID, created: map[string]bool{}}
	planCtx := context.WithValue(ctx, stackTokenCallKey{}, call)
	applyCtx := iac.WithStackScope(planCtx, iac.StackScope{StackID: shop.ID, StackName: shop.Name, Revision: 1})
	const prot = "/api/v1/apps/{name}/protection"
	for _, c := range []struct {
		name   string
		ctx    context.Context
		method string
		app    string
		want   int
	}{
		{"owned app", planCtx, http.MethodGet, "web", 0},
		{"owned app in apply", applyCtx, http.MethodGet, "web", 0},
		{"other stack app", planCtx, http.MethodGet, "blogapp", http.StatusNotFound},
		{"other stack app in own project", applyCtx, http.MethodGet, "shopblog", http.StatusNotFound},
		{"unowned app in project", planCtx, http.MethodGet, "loose", http.StatusNotFound},
		{"missing app", planCtx, http.MethodGet, "ghost", http.StatusNotFound},
		{"protect in apply", applyCtx, http.MethodPut, "web", http.StatusForbidden},
		{"unprotect in apply", applyCtx, http.MethodDelete, "web", http.StatusForbidden},
	} {
		rec := httptest.NewRecorder()
		body := ""
		if c.method == http.MethodPut {
			body = `{"reason":"x"}`
		}
		_, ok := rt.gateStackToken(rec, subRequest(c.ctx, tok, c.method+" "+prot, body, map[string]string{"name": c.app}), id)
		if c.want == 0 {
			if !ok {
				t.Errorf("%s: denied with %d", c.name, rec.Code)
			}
			continue
		}
		if ok || rec.Code != c.want {
			t.Errorf("%s: allowed=%v status %d, want %d", c.name, ok, rec.Code, c.want)
		}
	}

	// An app created in this call may be read; the mark does not leak to other names.
	call.markCreated(ledgerKeyOf(iac.KindApp, "fresh"))
	if _, ok := rt.gateStackToken(httptest.NewRecorder(), subRequest(applyCtx, tok, "GET "+prot, "", map[string]string{"name": "fresh"}), id); !ok {
		t.Error("protection of an app created in this call denied")
	}

	// Outside a stack call the route stays closed, as do unrelated routes.
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/apps/web/protection", ""},
		{http.MethodPut, "/api/v1/apps/web/protection", `{"reason":"x"}`},
		{http.MethodDelete, "/api/v1/apps/web/protection", ""},
		{http.MethodGet, "/api/v1/apps/web/secrets", ""},
		{http.MethodGet, "/api/v1/audit-log", ""},
	} {
		if code := callWithToken(t, rt, tok, c.method, c.path, c.body); code != http.StatusForbidden {
			t.Errorf("direct %s %s = %d, want 403", c.method, c.path, code)
		}
	}
}
