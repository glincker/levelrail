package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// stackLockFixture applies an app "web" under stack shop-prod with the given
// policy and returns a plain write token.
func stackLockFixture(t *testing.T, policy string) (*Router, *store.DB, *http.Cookie, string) {
	t.Helper()
	rt, db, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK {
		t.Fatalf("seed apply = %d", code)
	}
	if policy != "" {
		body := jsonBody(t, map[string]any{"drift_policy": policy})
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", body), nil); code != http.StatusOK {
			t.Fatalf("set policy = %d", code)
		}
	}
	plain := "write-secret-token"
	if err := db.SaveAPIToken(t.Context(), store.APIToken{ID: "w-1", Name: "writer", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWrite, AbilityDeploy}}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie, plain
}

type stackLockResp struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

func stackLockCall(t *testing.T, rt *Router, req *http.Request) (int, stackLockResp) {
	t.Helper()
	var out stackLockResp
	code := iacCall(t, rt, req, &out)
	return code, out
}

func TestStackLockBlocksConfigChangesAllowsOperations(t *testing.T) {
	rt, _, _, tok := stackLockFixture(t, "lock")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/apps/web", `{}`},
		{http.MethodPatch, "/api/v1/apps/web/domains", `{}`},
		{http.MethodDelete, "/api/v1/apps/web", ""},
		{http.MethodPut, "/api/v1/apps/web/health", `{}`},
		{http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2"}`},
	} {
		code, out := stackLockCall(t, rt, bearerRequest(c.method, c.path, c.body, tok))
		if code != http.StatusConflict || out.Code != "stack_locked" || !strings.Contains(out.Error, "shop-prod") {
			t.Errorf("%s %s = %d %+v, want 409 stack_locked naming the stack", c.method, c.path, code, out)
		}
	}
	for _, p := range []string{"restart", "start", "stop"} {
		if code, out := stackLockCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/apps/web/"+p, "", tok)); code == http.StatusConflict || out.Code == "stack_locked" {
			t.Errorf("POST %s was locked", p)
		}
	}
	if code := iacCall(t, rt, bearerRequest(http.MethodGet, "/api/v1/apps/web", "", tok), nil); code != http.StatusOK {
		t.Errorf("read = %d, want 200", code)
	}
}

func TestStackWarnPolicyAndUnownedResourcesAreNotBlocked(t *testing.T) {
	rt, _, _, tok := stackLockFixture(t, "")
	if code, out := stackLockCall(t, rt, bearerRequest(http.MethodPut, "/api/v1/apps/web", `{}`, tok)); out.Code == "stack_locked" {
		t.Fatalf("warn stack blocked: %d", code)
	}
	rt2, _, _, tok2 := stackLockFixture(t, "lock")
	if code, out := stackLockCall(t, rt2, bearerRequest(http.MethodPut, "/api/v1/apps/other", `{}`, tok2)); out.Code == "stack_locked" {
		t.Fatalf("unowned app blocked: %d", code)
	}
}

func TestStackOwnApplyWorksUnderLock(t *testing.T) {
	rt, _, cookie, _ := stackLockFixture(t, "lock")
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:2.0.0"), nil); code != http.StatusOK || out.Revision == nil || out.Revision.Status != store.StackRevApplied {
		t.Fatalf("apply under lock = %d %+v", code, out)
	}
}

func TestStackLockRootOverrideIsAudited(t *testing.T) {
	rt, db, cookie, tok := stackLockFixture(t, "lock")
	req := authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{}`)
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
	if code, out := stackLockCall(t, rt, req); out.Code == "stack_locked" {
		t.Fatalf("root override refused: %d", code)
	}
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{Action: "stack.lock_override"})
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Detail, "App/web") || entries[0].Path != "/api/v1/apps/web" {
		t.Fatalf("audit = %+v, %v", entries, err)
	}
	// Wrong stack name, and a non-root caller, are both ignored.
	req = authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{}`)
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "other")
	if _, out := stackLockCall(t, rt, req); out.Code != "stack_locked" {
		t.Error("override for another stack was honored")
	}
	req = bearerRequest(http.MethodPut, "/api/v1/apps/web", `{}`, tok)
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
	if _, out := stackLockCall(t, rt, req); out.Code != "stack_locked" {
		t.Error("override by a non-root token was honored")
	}
}

func TestStackLockOverrideIgnoredForCIAndStackTokens(t *testing.T) {
	rt, db, cookie, _ := stackLockFixture(t, "lock")
	ci := seedCIToken(t, db, "ci:acme/infra@refs/heads/main", []string{AbilityRoot})
	req := bearerRequest(http.MethodPut, "/api/v1/apps/web", `{}`, ci)
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
	if _, out := stackLockCall(t, rt, req); out.Code != "stack_locked" {
		t.Error("override honored for a CI token")
	}
	stackTok, _ := mintStackToken(t, rt, cookie, "shop-prod", applyVerbs, []string{AbilityRead, AbilityWrite})
	req = bearerRequest(http.MethodDelete, "/api/v1/apps/web", "", stackTok)
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
	if _, out := stackLockCall(t, rt, req); out.Code != "stack_locked" {
		t.Error("override honored for a stack token")
	}
	entries, _ := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{Action: "stack.lock_override"})
	if len(entries) != 0 {
		t.Errorf("refused overrides were audited as overrides: %d", len(entries))
	}
}

func TestStackForgetUnlocksResources(t *testing.T) {
	rt, _, cookie, tok := stackLockFixture(t, "lock")
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/shop-prod", ""), nil); code != http.StatusOK {
		t.Fatalf("forget = %d", code)
	}
	if _, out := stackLockCall(t, rt, bearerRequest(http.MethodPut, "/api/v1/apps/web", `{}`, tok)); out.Code == "stack_locked" {
		t.Fatal("forgotten stack still locks its apps")
	}
}

func TestStackLockTargets(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, "/api/v1/apps/web/restart", false},
		{http.MethodPost, "/api/v1/apps/web/secrets/K/rotate", true},
		{http.MethodPut, "/api/v1/apps/web/domains/a.com/headers", true},
		{http.MethodPut, "/api/v1/databases/db1/start", true},
		{http.MethodPost, "/api/v1/databases/db1/stop", false},
		{http.MethodPut, "/api/v1/environments/e1/env", true},
		{http.MethodPost, "/api/v1/projects/p1/environments", false},
		{http.MethodDelete, "/api/v1/domains/standalone/a.com", true},
		{http.MethodPut, "/api/v1/settings/oauth", false},
	} {
		if _, got := lockTargetsOf(c.method, c.path); got != c.want {
			t.Errorf("%s %s guarded = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
