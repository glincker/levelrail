package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeCIVerifier struct {
	claims   CIClaims
	err      error
	audience string
}

func (f *fakeCIVerifier) Verify(_ context.Context, _ string, audience string) (CIClaims, error) {
	f.audience = audience
	return f.claims, f.err
}

func ciExchange(t *testing.T, rt *Router, app string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/oidc-exchange", strings.NewReader(`{"app":"`+app+`","id_token":"jwt"}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestCIOIDCExchangeScopesTokenToTrustedApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "other")
	if err := db.SetDashboardURL(context.Background(), "https://cp.example.com"); err != nil {
		t.Fatal(err)
	}
	ver := &fakeCIVerifier{claims: CIClaims{Repository: "acme/site", Ref: "refs/heads/main", Subject: "repo:acme/site"}}
	rt.SetCIIDTokenVerifier(ver)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", `{"repository":"acme/site","ref_pattern":"refs/heads/main"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create trust = %d %s", rec.Code, rec.Body)
	}

	rec = ciExchange(t, rt, "web")
	if rec.Code != http.StatusOK {
		t.Fatalf("exchange = %d %s", rec.Code, rec.Body)
	}
	if ver.audience != "https://cp.example.com" {
		t.Fatalf("audience = %q", ver.audience)
	}
	var out ciExchangeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("response %s: %v", rec.Body, err)
	}

	get := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+out.Token)
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := get("/api/v1/apps/web/github-actions"); code != http.StatusOK {
		t.Fatalf("token on its own app = %d", code)
	}
	if code := get("/api/v1/apps/other/github-actions"); code == http.StatusOK {
		t.Fatalf("token reached an app it is not trusted for")
	}

	rec = ciExchange(t, rt, "other")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("exchange for untrusted app = %d", rec.Code)
	}
}

func TestCIOIDCExchangeRejections(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	ctx := context.Background()

	rec := ciExchange(t, rt, "web")
	if rec.Code != http.StatusConflict {
		t.Fatalf("no dashboard url = %d, want 409", rec.Code)
	}
	if err := db.SetDashboardURL(ctx, "https://cp.example.com"); err != nil {
		t.Fatal(err)
	}
	ver := &fakeCIVerifier{err: errors.New("bad signature")}
	rt.SetCIIDTokenVerifier(ver)
	if rec = ciExchange(t, rt, "web"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", `{"repository":"acme/site","ref_pattern":"refs/heads/release/*","environment":"production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create trust = %d %s", rec.Code, rec.Body)
	}
	cases := []struct {
		name   string
		claims CIClaims
	}{
		{"other repo", CIClaims{Repository: "evil/site", Ref: "refs/heads/release/1", Environment: "production"}},
		{"other ref", CIClaims{Repository: "acme/site", Ref: "refs/heads/main", Environment: "production"}},
		{"other environment", CIClaims{Repository: "acme/site", Ref: "refs/heads/release/1", Environment: "staging"}},
	}
	for _, tc := range cases {
		ver.err, ver.claims = nil, tc.claims
		if rec = ciExchange(t, rt, "web"); rec.Code != http.StatusForbidden {
			t.Fatalf("%s = %d, want 403", tc.name, rec.Code)
		}
	}
	ver.claims = CIClaims{Repository: "acme/site", Ref: "refs/heads/release/1", Environment: "production"}
	if rec = ciExchange(t, rt, "web"); rec.Code != http.StatusOK {
		t.Fatalf("matching claims = %d %s", rec.Code, rec.Body)
	}
}

func TestCITrustValidationAndWorkflow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	for _, body := range []string{
		`{"repository":"nope","ref_pattern":"refs/heads/main"}`,
		`{"repository":"acme/site","ref_pattern":"main"}`,
		`{"repository":"acme/site","ref_pattern":"refs/heads/[x"}`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", body, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/ci/workflow", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("workflow = %d %s", rec.Code, rec.Body)
	}
	var wf ciWorkflowResource
	if err := json.Unmarshal(rec.Body.Bytes(), &wf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id-token: write", "oidc: \"true\"", "rollback-on-failure", "[skip ci]", "app: web"} {
		if !strings.Contains(wf.YAML, want) {
			t.Errorf("workflow missing %q:\n%s", want, wf.YAML)
		}
	}
	if !wf.NeedsDomain || wf.HasTrust {
		t.Fatalf("flags = %+v", wf)
	}
}
