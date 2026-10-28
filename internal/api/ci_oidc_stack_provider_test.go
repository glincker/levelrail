package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
)

type stackProviderEnv struct {
	*oidcStackEnv
	idp *ciTestIDP
}

func newStackProviderEnv(t *testing.T) *stackProviderEnv {
	t.Helper()
	e := newOIDCStackEnv(t)
	idp := newCITestIDP(t)
	e.rt.SetCIIDTokenVerifier(&ciProviderTestVerifier{
		fakeCIVerifier: &fakeCIVerifier{err: errors.New("not github")},
		inner:          idp.verifier(t, testCIOIDCConfig(), allowAnyCIAddr), jwksURL: idp.srv.URL + "/keys",
	})
	return &stackProviderEnv{oidcStackEnv: e, idp: idp}
}

func (e *stackProviderEnv) exchangeGitLab(t *testing.T, mut func(map[string]any)) int {
	t.Helper()
	tok := e.idp.sign(t, jose.RS256, "rsa1", gitLabTestClaims(e.idp.srv.URL, mut))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/stack-oidc-exchange", strings.NewReader(jsonBody(t, map[string]string{"stack": "shop-prod", "id_token": tok})))
	return iacCall(t, e.rt, req, &stackExchangeResponse{})
}

func (e *stackProviderEnv) gitLabApplyTrust(t *testing.T, extra map[string]any) (int, stackTrustResource) {
	t.Helper()
	body := map[string]any{
		"provider": "gitlab", "issuer": e.idp.srv.URL, "repository": "acme/web", "ref_pattern": "refs/heads/main",
		"environment": "production", "verbs": []string{"plan", "apply"},
	}
	for k, v := range extra {
		body[k] = v
	}
	return e.addTrust(t, "shop-prod", body)
}

func TestStackOIDCGitLabApplyKeepsSafeDefaults(t *testing.T) {
	e := newStackProviderEnv(t)
	code, tr := e.gitLabApplyTrust(t, nil)
	if code != http.StatusCreated || tr.Provider != "gitlab" || !tr.ProtectedRefOnly || tr.Issuer != e.idp.srv.URL {
		t.Fatalf("create = %d %+v, want a protected gitlab trust", code, tr)
	}
	for name, mut := range map[string]func(map[string]any){
		"unprotected ref":           func(c map[string]any) { c["ref_protected"] = "false" },
		"merge request pipeline":    func(c map[string]any) { c["pipeline_source"] = "merge_request_event" },
		"external pull request":     func(c map[string]any) { c["pipeline_source"] = "external_pull_request_event" },
		"unprotected environment":   func(c map[string]any) { c["environment_protected"] = "false" },
		"other environment":         func(c map[string]any) { c["environment"] = "staging" },
		"other project":             func(c map[string]any) { c["project_path"] = "acme/other" },
		"issuer nobody configured":  func(c map[string]any) { c["iss"] = "https://gitlab.evil.example.com" },
		"tag where a branch is set": func(c map[string]any) { c["ref_type"] = "tag" },
	} {
		if got := e.exchangeGitLab(t, mut); got == http.StatusOK {
			t.Errorf("%s was exchanged for a stack token", name)
		}
	}
	if got := e.exchangeGitLab(t, nil); got != http.StatusOK {
		t.Fatalf("protected push on the trusted ref = %d, want 200", got)
	}
	exchanged := e.auditActions(t, auditStackOIDCExchange)
	if len(exchanged) != 1 || !strings.Contains(exchanged[0].Detail, `"provider":"gitlab"`) {
		t.Fatalf("exchange audit = %+v, want one row naming the provider", exchanged)
	}
	created := e.auditActions(t, auditStackTrustCreate)
	if len(created) != 1 || !strings.Contains(created[0].Detail, `"issuer"`) || !strings.Contains(created[0].Detail, `"protected_ref_only":true`) {
		t.Fatalf("trust audit = %+v, want issuer and protected_ref_only", created)
	}
}

func TestStackOIDCGitLabTrustValidation(t *testing.T) {
	e := newStackProviderEnv(t)
	for name, extra := range map[string]map[string]any{
		"apply without protected refs": {"protected_ref_only": false},
		"pull ref":                     {"ref_pattern": "refs/pull/*/merge", "verbs": []string{"plan"}},
		"apply without environment":    {"environment": ""},
		"http issuer":                  {"issuer": "http://gitlab.example.com"},
		"github style project":         {"repository": "web"},
		"subject pattern":              {"subject_pattern": "project_path:acme/web:*"},
		"unknown provider":             {"provider": "bitbucket"},
	} {
		if code, _ := e.gitLabApplyTrust(t, extra); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	code, tr := e.gitLabApplyTrust(t, map[string]any{"protected_ref_only": false, "verbs": []string{"plan"}, "ref_pattern": "refs/heads/*", "environment": ""})
	if code != http.StatusCreated || tr.ProtectedRefOnly || tr.Warning == "" {
		t.Fatalf("plan-only opt-out = %d %+v, want 201 with a warning", code, tr)
	}
	if code, _ := e.addTrust(t, "shop-prod", map[string]any{"repository": "acme/shop", "ref_pattern": "refs/heads/main", "issuer": "https://x.example.com"}); code != http.StatusBadRequest {
		t.Fatalf("github trust with an issuer = %d, want 400", code)
	}
}

func TestStackOIDCCircleCIApplyTrust(t *testing.T) {
	e := newStackProviderEnv(t)
	issuer := circleCIIssuerBase + ciTestOrg
	body := map[string]any{
		"provider": "circleci", "org_id": ciTestOrg, "repository": ciTestProject, "ref_pattern": "refs/heads/main",
		"environment": ciTestContext, "verbs": []string{"plan", "apply"},
	}
	code, tr := e.addTrust(t, "shop-prod", body)
	if code != http.StatusCreated || tr.Provider != "circleci" || tr.Issuer != issuer {
		t.Fatalf("create = %d %+v", code, tr)
	}
	tok := e.idp.sign(t, jose.RS256, "rsa1", circleCITestClaims(issuer, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ci/stack-oidc-exchange", strings.NewReader(jsonBody(t, map[string]string{"stack": "shop-prod", "id_token": tok})))
	if got := iacCall(t, e.rt, req, &stackExchangeResponse{}); got != http.StatusOK {
		t.Fatalf("circleci exchange = %d, want 200", got)
	}
	noCtx := e.idp.sign(t, jose.RS256, "rsa1", circleCITestClaims(issuer, func(c map[string]any) { c[circleCIClaimContextIDs] = []string{} }))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/ci/stack-oidc-exchange", strings.NewReader(jsonBody(t, map[string]string{"stack": "shop-prod", "id_token": noCtx})))
	if got := iacCall(t, e.rt, req, &stackExchangeResponse{}); got == http.StatusOK {
		t.Fatal("circleci job without the trusted context got an apply token")
	}
}

func TestStackOIDCProviderTrustNeedsRoot(t *testing.T) {
	e := newStackProviderEnv(t)
	tok := seedMatrixToken(t, e.db, "tok-deploy", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})
	body := jsonBody(t, map[string]any{"provider": "gitlab", "issuer": "https://gitlab.example.com", "repository": "acme/web", "ref_pattern": "refs/heads/main"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stacks/shop-prod/oidc-trusts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	bearer(tok)(req)
	if got := iacCall(t, e.rt, req, &stackTrustResource{}); got != http.StatusForbidden {
		t.Fatalf("non-root gitlab stack trust = %d, want 403", got)
	}
}
