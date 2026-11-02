package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func mintToken(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, createTokenResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body))
	var out createTokenResponse
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, out
}

func TestCreateToken_DefaultsToReadOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	code, got := mintToken(t, rt, cookie, `{"name":"mcp"}`)
	if code != http.StatusCreated {
		t.Fatalf("status = %d", code)
	}
	if !slices.Equal(got.Abilities, []string{AbilityRead}) || got.Access != tokenAccessReadOnly {
		t.Fatalf("abilities = %v access = %q, want read only", got.Abilities, got.Access)
	}
}

func TestCreateToken_AllowChangesIsExplicitOptIn(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	code, got := mintToken(t, rt, cookie, `{"name":"mcp-rw","allow_changes":true}`)
	if code != http.StatusCreated || !slices.Contains(got.Abilities, AbilityWrite) || got.Access != tokenAccessCanChange {
		t.Fatalf("status = %d abilities = %v access = %q, want write opt-in", code, got.Abilities, got.Access)
	}
	if code, _ := mintToken(t, rt, cookie, `{"name":"x","abilities":["read"],"allow_changes":true}`); code != http.StatusBadRequest {
		t.Fatalf("allow_changes with explicit abilities = %d, want 400", code)
	}
}

func TestCreateToken_ExistingTokenKeepsScopeWhenPolicyChanges(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if code, _ := mintToken(t, rt, cookie, `{"name":"old","abilities":["read","write"]}`); code != http.StatusCreated {
		t.Fatalf("status = %d", code)
	}
	path := rsBase + "/" + KeyMCPDefaultTokenScope
	if rec := rsDo(t, rt, cookie, http.MethodPut, path, `{"value":"read_write","confirm_weakening":true}`); rec.Code != http.StatusOK {
		t.Fatalf("set policy = %d %s", rec.Code, rec.Body.String())
	}
	_, rw := mintToken(t, rt, cookie, `{"name":"new-rw"}`)
	if !slices.Contains(rw.Abilities, AbilityWrite) {
		t.Fatalf("read_write policy abilities = %v, want write", rw.Abilities)
	}
	if rec := rsDo(t, rt, cookie, http.MethodPut, path, `{"value":"read_only"}`); rec.Code != http.StatusOK {
		t.Fatalf("tighten policy = %d", rec.Code)
	}
	rec := rsDo(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens", "")
	var list []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	byName := map[string]tokenResource{}
	for _, tk := range list {
		byName[tk.Name] = tk
	}
	if old := byName["old"]; !slices.Contains(old.Abilities, AbilityWrite) || old.Access != tokenAccessCanChange {
		t.Fatalf("old token = %+v, want unchanged write scope marked can_change", old)
	}
	if nw := byName["new-rw"]; !slices.Contains(nw.Abilities, AbilityWrite) {
		t.Fatalf("token minted under the old policy changed: %+v", nw)
	}
}

func TestDefaultTokenScopeSetting_WeakeningNeedsConfirm(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	path := rsBase + "/" + KeyMCPDefaultTokenScope
	steps := []struct {
		body string
		code int
	}{
		{`{"value":"read_write"}`, http.StatusBadRequest},
		{`{"value":"read_write","confirm_weakening":true}`, http.StatusOK},
		{`{"value":"read_only"}`, http.StatusOK},
		{`{"value":"admin"}`, http.StatusBadRequest},
	}
	for _, st := range steps {
		if rec := rsDo(t, rt, cookie, http.MethodPut, path, st.body); rec.Code != st.code {
			t.Fatalf("PUT %s = %d %s, want %d", st.body, rec.Code, rec.Body.String(), st.code)
		}
	}
}

func TestCreateToken_AllowChangesNeedsCallerWrite(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader-mcp@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	if code, _ := mintToken(t, rt, cookie, `{"name":"r","allow_changes":true}`); code != http.StatusForbidden {
		t.Fatalf("read-only caller allow_changes = %d, want 403", code)
	}
	if code, got := mintToken(t, rt, cookie, `{"name":"r"}`); code != http.StatusCreated || got.Access != tokenAccessReadOnly {
		t.Fatalf("read-only caller default = %d %+v, want read only token", code, got)
	}
}

func TestTokenAccess(t *testing.T) {
	for _, tc := range []struct {
		abilities []string
		want      string
	}{
		{[]string{AbilityRead}, tokenAccessReadOnly},
		{[]string{AbilityRead, AbilityReadSensitive}, tokenAccessReadOnly},
		{[]string{AbilityRead, AbilityDeploy}, tokenAccessCanChange},
		{[]string{AbilitySecretsReveal}, tokenAccessCanChange},
		{[]string{AbilityRoot}, tokenAccessCanChange},
	} {
		if got := tokenAccess(tc.abilities); got != tc.want {
			t.Errorf("tokenAccess(%v) = %q, want %q", tc.abilities, got, tc.want)
		}
	}
}

func TestTokenListAccess_PolicyAttachedIsCanChange(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	_, tk := mintToken(t, rt, cookie, `{"name":"scoped"}`)
	if tk.Access != tokenAccessReadOnly {
		t.Fatalf("fresh token access = %q", tk.Access)
	}
	attachTestPolicy(t, db, "allow-write", "Allow", "apps:write", "*", store.PrincipalTypeToken, tk.ID)
	rec := rsDo(t, rt, cookie, http.MethodGet, "/api/v1/auth/tokens", "")
	var list []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, got := range list {
		if got.ID == tk.ID && got.Access != tokenAccessCanChange {
			t.Fatalf("token with an attached policy shows %q, want can_change", got.Access)
		}
	}
}

func TestCreateToken_ReadWritePolicyCapsDefaultAtCallerAbilities(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	if rec := rsDo(t, rt, admin, http.MethodPut, rsBase+"/"+KeyMCPDefaultTokenScope, `{"value":"read_write","confirm_weakening":true}`); rec.Code != http.StatusOK {
		t.Fatalf("set policy = %d %s", rec.Code, rec.Body.String())
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader-cap@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	code, got := mintToken(t, rt, cookie, `{"name":"r"}`)
	if code != http.StatusCreated || !slices.Equal(got.Abilities, []string{AbilityRead}) {
		t.Fatalf("default for a reader = %d %v, want a read token", code, got.Abilities)
	}
	if code, _ := mintToken(t, rt, cookie, `{"name":"r2","abilities":["read","write"]}`); code != http.StatusForbidden {
		t.Fatalf("explicit write by a reader = %d, want 403", code)
	}
}
