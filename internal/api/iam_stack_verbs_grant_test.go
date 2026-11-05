package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStackVerbGrantNeedsGlobalRoot(t *testing.T) {
	rt, db, _ := stackFixture(t)
	op := storeUserWithAbilitiesForTest(t, db, "op@example.com", []string{AbilityRead, AbilityWrite})
	attachTestPolicy(t, db, "stack-root", "Allow", AbilityRoot, "stack:shop-prod", store.PrincipalTypeUser, op.ID)
	cookie := sessionCookieForTest(t, rt, op.ID)
	for _, c := range []struct {
		verbs []string
		want  int
	}{
		{[]string{"placement"}, http.StatusForbidden},
		{[]string{"plan", "apply"}, http.StatusCreated},
	} {
		body := jsonBody(t, map[string]any{"name": "ci", "abilities": []string{"read", "write"}, "stack": "shop-prod", "stack_verbs": c.verbs})
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body), nil); code != c.want {
			t.Errorf("mint %v = %d, want %d", c.verbs, code, c.want)
		}
	}

	e := newOIDCStackEnv(t)
	op2 := storeUserWithAbilitiesForTest(t, e.db, "op@example.com", []string{AbilityRead, AbilityWrite})
	attachTestPolicy(t, e.db, "stack-root", "Allow", AbilityRoot, "stack:shop-prod", store.PrincipalTypeUser, op2.ID)
	e.cookie = sessionCookieForTest(t, e.rt, op2.ID)
	trust := map[string]any{"repository": "acme/infra", "ref_pattern": "refs/heads/main", "environment": "prod", "verbs": []string{"placement"}}
	if code, _ := e.addTrust(t, "shop-prod", trust); code != http.StatusForbidden {
		t.Errorf("trust with placement by stack root = %d, want 403", code)
	}
	trust["verbs"] = []string{"plan", "apply"}
	if code, _ := e.addTrust(t, "shop-prod", trust); code != http.StatusCreated {
		t.Errorf("trust with apply by stack root = %d, want 201", code)
	}
}

func scopedStackCall(ctx context.Context, id string, stack store.StackRecord) context.Context {
	c := context.WithValue(ctx, stackTokenCallKey{}, &stackTokenCall{tokenID: id, stackID: stack.ID, created: map[string]bool{}})
	return iac.WithStackScope(c, iac.StackScope{StackID: stack.ID, StackName: stack.Name, Revision: 1})
}

func TestPlacementVerbProtectedToleration(t *testing.T) {
	t.Setenv("APP_NODES_PROTECTED_TAINT_KEYS", "backup")
	rt, db, cookie := stackFixture(t)
	ctx := t.Context()
	shop, _ := db.GetStackByName(ctx, "shop-prod")
	if err := db.UpsertStackResource(ctx, store.StackLedgerEntry{StackID: shop.ID, Kind: "App", Key: "App/web"}); err != nil {
		t.Fatal(err)
	}
	tok, id := mintStackToken(t, rt, cookie, "shop-prod", []string{"placement"}, []string{AbilityRead, AbilityWrite})
	sctx := scopedStackCall(ctx, id, shop)
	for _, c := range []struct {
		name, body string
		allow      bool
	}{
		{"no tolerations", `{}`, true},
		{"unprotected key", `{"tolerations":[{"key":"gpu","operator":"Exists"}]}`, true},
		{"protected key", `{"tolerations":[{"key":"backup","operator":"Exists"}]}`, false},
		{"protected key folded", `{"Tolerations":[{"Key":"backup","value":"yes"}]}`, false},
		{"wildcard", `{"tolerations":[{"operator":"Exists"}]}`, false},
		{"duplicate keys", `{"tolerations":[],"Tolerations":[{"key":"backup"}]}`, false},
	} {
		req := subRequest(sctx, tok, "PUT /api/v1/apps/{name}/placement", c.body, map[string]string{"name": "web"})
		if _, ok := rt.gateStackToken(httptest.NewRecorder(), req, id); ok != c.allow {
			t.Errorf("%s: allowed = %v, want %v", c.name, ok, c.allow)
		}
	}
}

func TestStackSettingVerbNeedsRiskKey(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	ctx := t.Context()
	createInstallStack(t, rt, cookie, "nodes-eu")
	eu, _ := db.GetStackByName(ctx, "nodes-eu")
	tok, id := mintStackToken(t, rt, cookie, "nodes-eu", []string{"setting"}, []string{AbilityRead, AbilityWrite})
	sctx := scopedStackCall(ctx, id, eu)
	for _, c := range []struct {
		key, body string
		allow     bool
	}{
		{runtimesettings.KeyDoctorReachabilityDialbackURL, `{"value":"https://example.com/x"}`, false},
		{runtimesettings.KeyTrustLoopbackForwarded, `{"value":true}`, false},
		{runtimesettings.KeyDoctorReachabilityDialbackAllowPrivate, `{"value":false}`, true},
	} {
		req := subRequest(sctx, tok, "PUT /api/v1/settings/runtime/{key}", c.body, map[string]string{"key": c.key})
		if _, ok := rt.gateStackToken(httptest.NewRecorder(), req, id); ok != c.allow {
			t.Errorf("%s: allowed = %v, want %v", c.key, ok, c.allow)
		}
	}
}

func TestStackLockUnauthenticated401(t *testing.T) {
	rt, db, _, _ := stackLockFixture(t, "lock")
	rt.policies = brokenLedger{db}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/runtime/free.key", nil)
	if code, out := stackLockCall(t, rt, req); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated setting write on ledger error = %d %+v, want 401", code, out)
	}
}
