package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_IAMSimulate(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"principal_type":"token","principal_id":"t1","principal_name":"ci","action":"write","resource":"app:web","allowed":false,"decided_by":"explicit_deny","deciding_statement":{"policy_id":"p1","policy_name":"no-prod","statement_index":0,"effect":"Deny","action":["write"],"resource":["app:web"]},"matched_statements":[],"base_ability_grants":true,"environment_resources":[]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "simulate", "--api-url", srv.URL, "--principal", "token:t1", "--action", "write", "--resource", "app:web"})
	if !strings.HasPrefix(gotQuery, "/api/v1/iam/simulate?") || !strings.Contains(gotQuery, "principal_id=t1") || !strings.Contains(gotQuery, "resource=app%3Aweb") {
		t.Errorf("request = %q", gotQuery)
	}
	if !strings.Contains(stdout, "DENY") || !strings.Contains(stdout, "no-prod") {
		t.Errorf("stdout = %q, want the verdict and deciding policy", stdout)
	}
}

func TestRun_IAMSimulate_RequiresPrincipal(t *testing.T) {
	_ = runCLIExpectValidationError(t, []string{"iam", "simulate", "--principal", "group:x", "--action", "read", "--resource", "app:web"})
}

func TestRun_IAMAnalyze_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/iam/analyze" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"score":70,"counts":{"critical":1},"findings":[{"kind":"allow_all","severity":"critical","message":"allows everything","fix":"narrow it"}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "analyze", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"score": 70`) || !strings.Contains(stdout, `"allow_all"`) {
		t.Errorf("stdout = %q", stdout)
	}
}
