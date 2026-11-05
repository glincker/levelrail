package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/appwizard"
)

func TestRepoPlan_ReturnsPlanAndCreatesNothing(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{
		"package.json": `{"name":"a","dependencies":{"express":"4"}}`,
		".env.example": "API_TOKEN=s3cr3t=value\n",
	}})
	before, _ := rt.apps.ListDesiredServices(t.Context())
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/repo-plan", `{"repo_url":"https://github.com/acme/a","ref":"main","token":"ghp_supersecret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{"ghp_supersecret", "s3cr3t"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("plan leaks %q: %s", leak, rec.Body.String())
		}
	}
	var pv appwizard.PlanView
	if err := json.Unmarshal(rec.Body.Bytes(), &pv); err != nil {
		t.Fatal(err)
	}
	if pv.Next.Method != http.MethodPost || pv.Next.Path != "/api/v1/imports/repo/apply" || len(pv.Env) != 1 || !pv.Env[0].LooksSecret || pv.Spec.YAML == "" {
		t.Fatalf("plan = %+v", pv)
	}
	after, _ := rt.apps.ListDesiredServices(t.Context())
	if len(after) != len(before) {
		t.Fatalf("plan created rows: %d -> %d", len(before), len(after))
	}
}

func TestRepoPlan_Authorization(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{"package.json": `{"name":"a"}`}})
	do := func(tok string) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/apps/repo-plan", strings.NewReader(`{"repo_url":"https://github.com/acme/a","ref":"main"}`))
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do(""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", code)
	}
	if code := do(seedMatrixToken(t, db, "ro", []string{AbilityRead})); code != http.StatusForbidden {
		t.Fatalf("read-only token = %d, want 403", code)
	}
	if code := do(seedMatrixToken(t, db, "wr", []string{AbilityRead, AbilityWrite})); code != http.StatusOK {
		t.Fatalf("write token = %d, want 200", code)
	}
}

func TestRepoPlan_BadInput(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	for name, body := range map[string]string{"empty": `{}`, "scheme": `{"repo_url":"ftp://x/y"}`, "json": `{`} {
		if rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/repo-plan", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", name, rec.Code)
		}
	}
}
