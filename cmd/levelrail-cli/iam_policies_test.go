package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPolicyDocumentJSON = `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`

func TestRun_IAMPoliciesCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody policyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(policyResource{ID: "pol_1", Name: gotBody.Name, Document: gotBody.Document})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"iam", "policies", "create",
		"--name", "web-readers", "--document", testPolicyDocumentJSON,
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/iam/policies" {
		t.Errorf("request = %s %s, want POST /api/v1/iam/policies", gotMethod, gotPath)
	}
	if gotBody.Name != "web-readers" {
		t.Errorf("request body name = %q, want web-readers", gotBody.Name)
	}
	if !strings.Contains(stdout, "web-readers") || !strings.Contains(stdout, "pol_1") {
		t.Errorf("stdout = %q, want a created confirmation", stdout)
	}
}

func TestRun_IAMPoliciesCreate_DocumentFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(path, []byte(testPolicyDocumentJSON), 0o600); err != nil {
		t.Fatalf("write policy file: %v", err)
	}

	var gotBody policyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(policyResource{ID: "pol_1", Name: gotBody.Name, Document: gotBody.Document})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"iam", "policies", "create",
		"--name", "from-file", "--document", "file://" + path,
		"--api-url", srv.URL,
	})
	if !strings.Contains(string(gotBody.Document), "app:web") {
		t.Errorf("request body document = %s, want the file's contents", gotBody.Document)
	}
}

func TestRun_IAMPoliciesCreate_MissingName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"iam", "policies", "create", "--document", testPolicyDocumentJSON}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_IAMPoliciesCreate_MissingDocument(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"iam", "policies", "create", "--name", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_IAMPoliciesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []policyResource{{ID: "pol_1", Name: "web-readers", Description: "read access"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "policies", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/iam/policies" {
		t.Errorf("path = %q, want /api/v1/iam/policies", gotPath)
	}
	if !strings.Contains(stdout, "web-readers") || !strings.Contains(stdout, "read access") {
		t.Errorf("stdout = %q, want the policy name and description", stdout)
	}
}

func TestRun_IAMPoliciesList_JSON(t *testing.T) {
	srv := newListEchoServer(t, nil, []policyResource{{ID: "pol_1", Name: "web-readers"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "policies", "list", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"id": "pol_1"`) {
		t.Errorf("stdout = %q, want the policy as JSON", stdout)
	}
}

func TestRun_IAMPoliciesGet(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(policyResource{ID: "pol_1", Name: "web-readers", Document: json.RawMessage(testPolicyDocumentJSON)})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "policies", "get", "pol_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/iam/policies/pol_1" {
		t.Errorf("path = %q, want /api/v1/iam/policies/pol_1", gotPath)
	}
	if !strings.Contains(stdout, "web-readers") {
		t.Errorf("stdout = %q, want the policy name", stdout)
	}
}

func TestRun_IAMPoliciesUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody policyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(policyResource{ID: "pol_1", Name: gotBody.Name, Document: gotBody.Document})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"iam", "policies", "update", "pol_1",
		"--name", "renamed", "--document", testPolicyDocumentJSON,
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/iam/policies/pol_1" {
		t.Errorf("request = %s %s, want PUT /api/v1/iam/policies/pol_1", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "renamed") {
		t.Errorf("stdout = %q, want an updated confirmation", stdout)
	}
}

func TestRun_IAMPoliciesDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "policies", "delete", "pol_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/iam/policies/pol_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/iam/policies/pol_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, "deleted") {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_IAMPoliciesAttach(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody attachPolicyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"iam", "policies", "attach", "pol_1",
		"--principal-type", "user", "--principal-id", "user_1",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/iam/policies/pol_1/attachments" {
		t.Errorf("request = %s %s, want POST /api/v1/iam/policies/pol_1/attachments", gotMethod, gotPath)
	}
	if gotBody.PrincipalType != "user" || gotBody.PrincipalID != "user_1" {
		t.Errorf("request body = %+v", gotBody)
	}
}

func TestRun_IAMPoliciesAttach_InvalidPrincipalType(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{
		"iam", "policies", "attach", "pol_1",
		"--principal-type", "robot", "--principal-id", "x",
	}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_IAMPoliciesDetach(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"iam", "policies", "detach", "pol_1",
		"--principal-type", "token", "--principal-id", "tok_1",
		"--api-url", srv.URL,
	})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/iam/policies/pol_1/attachments/token/tok_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/iam/policies/pol_1/attachments/token/tok_1", *gotMethod, *gotPath)
	}
}

func TestRun_IAMPoliciesAttachments(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []policyAttachmentResource{{PrincipalType: "user", PrincipalID: "user_1"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"iam", "policies", "attachments", "pol_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/iam/policies/pol_1/attachments" {
		t.Errorf("path = %q, want /api/v1/iam/policies/pol_1/attachments", gotPath)
	}
	if !strings.Contains(stdout, "user_1") {
		t.Errorf("stdout = %q, want the attached principal id", stdout)
	}
}

func TestRun_IAMPolicies_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusForbidden, `{"error":"your account lacks the required ability"}`)
	stderr := runCLIExpectAPIError(t, []string{"iam", "policies", "list", "--api-url", srv.URL})
	if !strings.Contains(stderr, "lacks the required ability") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_IAMPolicies_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"iam", "policies", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_IAM_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"iam", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
