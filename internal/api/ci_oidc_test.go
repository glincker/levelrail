package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/GLINCKER/levelrail/internal/store"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

type ciProviderTestVerifier struct {
	*fakeCIVerifier
	inner   *ciJWTVerifier
	jwksURL string
	specs   []ciProviderSpec
}

// VerifyProvider records the spec the router derived, then points the fetch at the fake JWKS server.
func (v *ciProviderTestVerifier) VerifyProvider(ctx context.Context, raw string, spec ciProviderSpec, audience string) (CIClaims, error) {
	v.specs = append(v.specs, spec)
	spec.JWKSURL = v.jwksURL
	return v.inner.VerifyProvider(ctx, raw, spec, audience)
}

func TestCIOIDCExchangeGitLabAndCircleCI(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := db.SetDashboardURL(context.Background(), ciTestAudience); err != nil {
		t.Fatal(err)
	}
	idp := newCITestIDP(t)
	pv := &ciProviderTestVerifier{fakeCIVerifier: &fakeCIVerifier{err: errors.New("not github")}, inner: idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr), jwksURL: idp.srv.URL + "/keys"}
	rt.SetCIIDTokenVerifier(pv)
	post := func(path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		if cookie != nil {
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
		} else {
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		}
		return rec
	}
	exchange := func(tok string) int {
		return post("/api/v1/ci/oidc-exchange", `{"app":"web","id_token":"`+tok+`"}`, nil).Code
	}

	for _, body := range []string{
		`{"provider":"bitbucket","repository":"acme/web","ref_pattern":"refs/heads/main"}`,
		`{"provider":"gitlab","issuer":"http://gitlab.example.com","repository":"acme/web","ref_pattern":"refs/heads/main"}`,
		`{"provider":"gitlab","issuer":"https://oidc.circleci.com/org/x","repository":"acme/web","ref_pattern":"refs/heads/main"}`,
		`{"provider":"gitlab","repository":"web","ref_pattern":"refs/heads/main"}`,
		`{"provider":"circleci","repository":"` + ciTestProject + `","ref_pattern":"refs/heads/main"}`,
		`{"provider":"circleci","org_id":"` + ciTestOrg + `","repository":"acme/web","ref_pattern":"refs/heads/main"}`,
		`{"provider":"circleci","org_id":"` + ciTestOrg + `","repository":"` + ciTestProject + `","ref_pattern":"refs/heads/main","protected_ref_only":true}`,
		`{"repository":"acme/web","issuer":"https://gitlab.com","ref_pattern":"refs/heads/main"}`,
	} {
		if rec := post("/api/v1/apps/web/ci/trusts", body, cookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", body, rec.Code)
		}
	}
	for _, body := range []string{
		`{"provider":"gitlab","issuer":"` + idp.srv.URL + `/","repository":"acme/web","ref_pattern":"refs/heads/main","environment":"production","protected_ref_only":true}`,
		`{"provider":"circleci","org_id":"` + ciTestOrg + `","repository":"` + ciTestProject + `","ref_pattern":"refs/heads/main","environment":"` + ciTestContext + `"}`,
		`{"repository":"acme/other","ref_pattern":"refs/heads/main"}`,
	} {
		if rec := post("/api/v1/apps/web/ci/trusts", body, cookie); rec.Code != http.StatusCreated {
			t.Fatalf("create %s = %d %s", body, rec.Code, rec.Body)
		}
	}

	gitlab := func(mut func(map[string]any)) string {
		return idp.sign(t, jose.RS256, "rsa1", gitLabTestClaims(idp.srv.URL, mut))
	}
	circle := func(mut func(map[string]any)) string {
		return idp.sign(t, jose.ES256, "ec1", circleCITestClaims(circleCIIssuerBase+ciTestOrg, mut))
	}
	cases := []struct {
		name string
		tok  string
		want int
	}{
		{"gitlab match", gitlab(nil), http.StatusOK},
		{"gitlab unprotected ref", gitlab(func(c map[string]any) { c["ref_protected"] = "false" }), http.StatusForbidden},
		{"gitlab other project", gitlab(func(c map[string]any) { c["project_path"] = "acme/other" }), http.StatusForbidden},
		{"gitlab other branch", gitlab(func(c map[string]any) { c["ref"] = "dev" }), http.StatusForbidden},
		{"gitlab other environment", gitlab(func(c map[string]any) { c["environment"] = "staging" }), http.StatusForbidden},
		{"gitlab wrong audience", gitlab(func(c map[string]any) { c["aud"] = "https://elsewhere.example.com" }), http.StatusUnauthorized},
		{"gitlab expired", gitlab(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }), http.StatusUnauthorized},
		{"circleci match", circle(nil), http.StatusOK},
		{"circleci without the context", circle(func(c map[string]any) { c[circleCIClaimContextIDs] = []string{} }), http.StatusForbidden},
		{"circleci other branch", circle(func(c map[string]any) { c[circleCIClaimVCSRef] = "refs/heads/dev" }), http.StatusForbidden},
		{"circleci no vcs ref", circle(func(c map[string]any) { delete(c, circleCIClaimVCSRef) }), http.StatusForbidden},
	}
	for _, tc := range cases {
		if got := exchange(tc.tok); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, got, tc.want)
		}
	}
	for _, s := range pv.specs {
		want := s.Issuer + gitLabJWKSPath
		if s.Provider == ciProviderCircleCI {
			want = circleCIIssuerBase + ciTestOrg + circleCIJWKSPath
		}
		if s.JWKSURL != want || (s.Issuer != idp.srv.URL && s.Issuer != circleCIIssuerBase+ciTestOrg) {
			t.Fatalf("derived spec %+v", s)
		}
	}

	seen, fetched := len(pv.specs), idp.fetches.Load()
	stranger := idp.sign(t, jose.RS256, "rsa1", gitLabTestClaims("https://gitlab.evil.example", nil))
	if got := exchange(stranger); got != http.StatusUnauthorized {
		t.Fatalf("issuer not on the allowlist = %d, want 401", got)
	}
	if len(pv.specs) != seen || idp.fetches.Load() != fetched {
		t.Fatal("an issuer no trust names reached the verifier or the JWKS endpoint")
	}
}

func TestCITrustMatchesKeepsProvidersApart(t *testing.T) {
	t.Parallel()
	github := store.CIOIDCTrust{Provider: ciProviderGitHub, Repository: "acme/web", RefPattern: "refs/heads/main"}
	gitlab := store.CIOIDCTrust{Provider: ciProviderGitLab, Issuer: "https://gitlab.com", Repository: "acme/web", RefPattern: "refs/heads/main"}
	glClaims := CIClaims{Provider: ciProviderGitLab, Issuer: "https://gitlab.com", Repository: "acme/web", Ref: "refs/heads/main"}
	if ciTrustMatches(github, glClaims) {
		t.Fatal("gitlab identity matched a github trust")
	}
	if ciTrustMatches(gitlab, CIClaims{Repository: "acme/web", Ref: "refs/heads/main"}) {
		t.Fatal("github identity matched a gitlab trust")
	}
	other := glClaims
	other.Issuer = "https://gitlab.example.com"
	if ciTrustMatches(gitlab, other) {
		t.Fatal("identity from another gitlab instance matched")
	}
	if !ciTrustMatches(gitlab, glClaims) || !ciTrustMatches(store.CIOIDCTrust{Repository: "acme/web", RefPattern: "refs/heads/*"}, CIClaims{Repository: "acme/web", Ref: "refs/heads/x"}) {
		t.Fatal("matching identities refused")
	}
}
