package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_IAMTemplatesList(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewEncoder(w).Encode(apiclient.PolicyTemplateList{Version: 1, Templates: []apiclient.PolicyTemplate{
			{ID: "guest-one-environment", Name: "Guest", Description: "d", Params: []apiclient.PolicyTemplateParam{{Name: "environment", Required: true}}},
		}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"iam", "templates", "list", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/iam/policy-templates" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "guest-one-environment") || !strings.Contains(stdout, "environment") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_IAMTemplatesApply(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody apiclient.ApplyPolicyTemplateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.ApplyPolicyTemplateResponse{Policy: apiclient.PolicyResource{ID: "pol_1", Name: "guest-one-environment-env_dev"}, Attached: gotBody.Attach != nil})
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		wantType   string
		wantID     string
		wantParams map[string]string
	}{
		{"attach user", []string{"--param", "environment=env_dev", "--attach-user", "user_1"}, "user", "user_1", map[string]string{"environment": "env_dev"}},
		{"attach token", []string{"--param", "environment=env_dev", "--attach-token", "tok_1"}, "token", "tok_1", map[string]string{"environment": "env_dev"}},
		{"no attach", []string{"--param", "environment=env_dev"}, "", "", map[string]string{"environment": "env_dev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBody = apiclient.ApplyPolicyTemplateRequest{}
			args := append([]string{"iam", "templates", "apply", "guest-one-environment", "--api-url", srv.URL}, tt.args...)
			stdout, _ := runCLIExpectOK(t, args)
			if gotMethod != http.MethodPost || gotPath != "/api/v1/iam/policy-templates/guest-one-environment/apply" {
				t.Errorf("request = %s %s", gotMethod, gotPath)
			}
			if gotBody.Params["environment"] != "env_dev" {
				t.Errorf("params = %v", gotBody.Params)
			}
			if tt.wantType == "" {
				if gotBody.Attach != nil {
					t.Errorf("attach = %+v, want nil", gotBody.Attach)
				}
			} else if gotBody.Attach == nil || gotBody.Attach.PrincipalType != tt.wantType || gotBody.Attach.PrincipalID != tt.wantID {
				t.Errorf("attach = %+v", gotBody.Attach)
			}
			if !strings.Contains(stdout, "pol_1") {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}

func TestRun_IAMTemplatesApply_Validation(t *testing.T) {
	tests := [][]string{
		{"iam", "templates", "apply", "read-only", "--param", "novalue"},
		{"iam", "templates", "apply", "read-only", "--attach-user", "u", "--attach-token", "t"},
	}
	for _, args := range tests {
		runCLIExpectValidationError(t, append(args, "--api-url", "http://127.0.0.1:1"))
	}
}

func TestParseTemplateParams(t *testing.T) {
	got, err := parseTemplateParams([]string{"a=b", "c=d=e"})
	if err != nil || got["a"] != "b" || got["c"] != "d=e" {
		t.Fatalf("got %v, err %v", got, err)
	}
	for _, bad := range []string{"a", "=b", "a="} {
		if _, err := parseTemplateParams([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
