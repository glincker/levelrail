package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// liveTestSignIn runs the dashboard's Test sign-in against a real IdP and returns the stored result.
func (h *samlHarness) liveTestSignIn(idp liveIDP, user, pass string, tamper func(string) string) ssoTestResult {
	h.t.Helper()
	req := authedRequest(h.t, h.admin, http.MethodGet, "/api/v1/sso/saml/connections/"+h.connID+"/test/start", "")
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		h.t.Fatalf("test start: %d %s", rec.Code, rec.Body)
	}
	var flow *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == samlFlowCookie {
			flow = c
		}
	}
	if flow == nil {
		h.t.Fatal("test start set no flow cookie")
	}
	resp := idp.login(h.t, rec.Header().Get("Location"), user, pass)
	if tamper != nil {
		resp = tamper(resp)
	}
	back := h.post(flow, resp)
	if want := samlTestReturn(h.connID); back.Header().Get("Location") != want {
		h.t.Fatalf("ACS redirect = %q, want %q", back.Header().Get("Location"), want)
	}
	res := h.admin2(http.MethodGet, "/api/v1/sso/saml/connections/"+h.connID+"/test/result", nil)
	var out ssoTestResult
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		h.t.Fatal(err)
	}
	if again := h.admin2(http.MethodGet, "/api/v1/sso/saml/connections/"+h.connID+"/test/result", nil); again.Body.String() == res.Body.String() {
		h.t.Fatal("a test result can be read twice")
	}
	return out
}

func TestLiveSAMLTestSignInAndDomainRouting(t *testing.T) {
	skipUnlessLiveIDP(t)
	kc := newLiveKeycloak(t)

	t.Run("test sign-in reports claims and the mapped role without creating anything", func(t *testing.T) {
		h := newLiveHarness(t, kc, liveSignAssertion, map[string]any{"allowed_domains": []string{"example.com"}})
		if rec := h.admin2(http.MethodPut, "/api/v1/sso/group-mappings", map[string]any{"mappings": []map[string]any{{"group": "Engineering", "role_id": "role_operator", "priority": 5}}}); rec.Code != http.StatusOK {
			t.Fatalf("mappings: %d %s", rec.Code, rec.Body)
		}
		res := h.liveTestSignIn(kc, "ada", "ada-pass", nil)
		ev := res.Evaluation
		if res.Status != ssoTestStatusOK || ev == nil || !ev.Allowed || ev.Email != "ada@example.com" || ev.RoleID != "role_operator" ||
			ev.RoleSource != ssoRoleSourceMapping || ev.AccountState != ssoAccountNew || len(ev.Groups) != 1 || ev.Groups[0] != "Engineering" {
			t.Fatalf("result = %+v evaluation = %+v", res, ev)
		}
		if _, err := h.db.GetUserByEmail(context.Background(), "ada@example.com"); err == nil {
			t.Fatal("a test sign-in created an account")
		}
	})

	t.Run("a domain the connection does not claim is a typed failure with an auto fix", func(t *testing.T) {
		h := newLiveHarness(t, kc, liveSignAssertion, map[string]any{"allowed_domains": []string{"lab.example"}})
		res := h.liveTestSignIn(kc, "ada", "ada-pass", nil)
		if res.Evaluation == nil || res.Evaluation.Allowed || res.Evaluation.RefusalCode != samlErrDomain ||
			len(res.Evaluation.Issues) == 0 || res.Evaluation.Issues[0].Fix.Action != "add_domain" || res.Evaluation.Issues[0].Fix.Value != "example.com" {
			t.Fatalf("evaluation = %+v", res.Evaluation)
		}
	})

	t.Run("a tampered response reports signature_invalid", func(t *testing.T) {
		h := newLiveHarness(t, kc, liveSignAssertion, nil)
		res := h.liveTestSignIn(kc, "ada", "ada-pass", func(resp string) string {
			return tamperSAMLResponse(t, resp, "ada@example.com", "eve@example.com")
		})
		if res.Status != ssoTestStatusFailed || res.FailureCode != samlFailSignature {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("login routing sends a claimed domain to its connection and nothing else", func(t *testing.T) {
		h := newLiveHarness(t, kc, liveSignAssertion, map[string]any{"allowed_domains": []string{"example.com"}})
		route := func(domain string) (int, *ssoRoute) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login-options?sso_domain="+url.QueryEscape(domain), nil)
			req.RemoteAddr = "127.0.0.1:50000"
			rec := httptest.NewRecorder()
			h.rt.Handler().ServeHTTP(rec, req)
			var body struct {
				SSORoute *ssoRoute `json:"sso_route"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			return rec.Code, body.SSORoute
		}
		if code, r := route("example.com"); code != http.StatusOK || r == nil || r.Kind != ssoKindSAML || r.ID != h.connID {
			t.Fatalf("claimed domain: %d %+v", code, r)
		}
		if code, r := route("unclaimed.test"); code != http.StatusOK || r != nil {
			t.Fatalf("unclaimed domain: %d %+v", code, r)
		}
	})
}

func TestClassifySAMLFailure(t *testing.T) {
	tests := []struct{ reason, want string }{
		{"audience is not this service provider", samlFailAudience},
		{"Signature could not be verified", samlFailSignature},
		{"assertion not signed", samlFailSignature},
		{"expired at 2026-01-01", samlFailClockSkew},
		{"InResponseTo does not match", samlFailRequest},
		{"malformed xml", samlFailUnreadable},
	}
	for _, tc := range tests {
		if got := classifySAMLFailure(errors.New(tc.reason)); got != tc.want {
			t.Errorf("%q = %s, want %s", tc.reason, got, tc.want)
		}
	}
}
