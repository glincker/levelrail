package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
)

type ciGitLabHarness struct {
	t      *testing.T
	rt     *Router
	cookie *http.Cookie
	idp    *ciTestIDP
}

func newCIGitLabHarness(t *testing.T, apps ...string) *ciGitLabHarness {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, a := range apps {
		seedApp(t, db, a)
	}
	if err := db.SetDashboardURL(context.Background(), ciTestAudience); err != nil {
		t.Fatal(err)
	}
	idp := newCITestIDP(t)
	rt.SetCIIDTokenVerifier(&ciProviderTestVerifier{fakeCIVerifier: &fakeCIVerifier{err: errors.New("not github")}, inner: idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr), jwksURL: idp.srv.URL + "/keys"})
	return &ciGitLabHarness{t: t, rt: rt, cookie: cookie, idp: idp}
}

func (h *ciGitLabHarness) createTrust(app, body string) (int, ciTrustResource) {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, authedRequest(h.t, h.cookie, http.MethodPost, "/api/v1/apps/"+app+"/ci/trusts", body))
	var out ciTrustResource
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (h *ciGitLabHarness) exchange(app string, mut func(map[string]any)) int {
	h.t.Helper()
	tok := h.idp.sign(h.t, jose.RS256, "rsa1", gitLabTestClaims(h.idp.srv.URL, mut))
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ci/oidc-exchange", strings.NewReader(`{"app":"`+app+`","id_token":"`+tok+`"}`)))
	return rec.Code
}

func TestCITrustGitLabProtectedRefByDefault(t *testing.T) {
	h := newCIGitLabHarness(t, "web", "api")
	code, tr := h.createTrust("web", `{"provider":"gitlab","issuer":"`+h.idp.srv.URL+`","repository":"acme/web","ref_pattern":"refs/heads/main"}`)
	if code != http.StatusCreated || !tr.ProtectedRefOnly || tr.Warning != "" {
		t.Fatalf("default gitlab trust = %d %+v, want protected_ref_only and no warning", code, tr)
	}
	// A fork MR run in the parent project, or any developer's unprotected branch named main.
	if got := h.exchange("web", func(c map[string]any) { c["ref_protected"] = "false" }); got == http.StatusOK {
		t.Fatal("unprotected ref deployed through a default gitlab trust")
	}
	if got := h.exchange("web", func(c map[string]any) { c["ref_protected"] = true; delete(c, "environment_protected") }); got != http.StatusOK {
		t.Fatalf("protected ref as a JSON bool = %d, want 200", got)
	}

	code, tr = h.createTrust("api", `{"provider":"gitlab","issuer":"`+h.idp.srv.URL+`","repository":"acme/web","ref_pattern":"refs/heads/main","protected_ref_only":false}`)
	if code != http.StatusCreated || tr.ProtectedRefOnly || tr.Warning == "" {
		t.Fatalf("opt-out gitlab trust = %d %+v, want a warning", code, tr)
	}
	if got := h.exchange("api", func(c map[string]any) { c["ref_protected"] = "false" }); got != http.StatusOK {
		t.Fatalf("explicit opt-out refused an unprotected ref = %d", got)
	}
}

func TestCIOIDCGitLabRefusesMergeRequestPipelines(t *testing.T) {
	h := newCIGitLabHarness(t, "web")
	if code, _ := h.createTrust("web", `{"provider":"gitlab","issuer":"`+h.idp.srv.URL+`","repository":"acme/web","ref_pattern":"refs/heads/main","protected_ref_only":false}`); code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	for _, src := range []string{"merge_request_event", "external_pull_request_event"} {
		if got := h.exchange("web", func(c map[string]any) { c["pipeline_source"] = src }); got == http.StatusOK {
			t.Fatalf("pipeline_source %s deployed", src)
		}
	}
	if got := h.exchange("web", nil); got != http.StatusOK {
		t.Fatalf("push pipeline = %d, want 200", got)
	}
}

func TestCIOIDCGitLabEnvironmentMustBeProtected(t *testing.T) {
	h := newCIGitLabHarness(t, "web")
	if code, _ := h.createTrust("web", `{"provider":"gitlab","issuer":"`+h.idp.srv.URL+`","repository":"acme/web","ref_pattern":"refs/heads/main","environment":"production"}`); code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	cases := []struct {
		name string
		mut  func(map[string]any)
		want bool
	}{
		{"string true", nil, true},
		{"bool true", func(c map[string]any) { c["environment_protected"] = true }, true},
		{"missing", func(c map[string]any) { delete(c, "environment_protected") }, false},
		{"string false", func(c map[string]any) { c["environment_protected"] = "false" }, false},
		{"bool false", func(c map[string]any) { c["environment_protected"] = false }, false},
	}
	for _, tc := range cases {
		if got := h.exchange("web", tc.mut) == http.StatusOK; got != tc.want {
			t.Errorf("environment_protected %s: deployed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
