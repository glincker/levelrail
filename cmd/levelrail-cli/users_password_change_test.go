package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_UsersRequirePasswordChange(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"user_1","email":"pat@example.com","must_change_password":` + map[bool]string{true: "true", false: "false"}[gotBody["required"]] + `}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "require-password-change", "user_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/users/user_1/password-change-required" || !gotBody["required"] {
		t.Fatalf("request = %s %s %v", gotMethod, gotPath, gotBody)
	}
	if !strings.Contains(stdout, "must change their password") {
		t.Errorf("stdout = %q", stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"users", "require-password-change", "user_1", "--off", "--api-url", srv.URL})
	if gotBody["required"] || !strings.Contains(stdout, "no longer") {
		t.Errorf("--off: body %v stdout %q", gotBody, stdout)
	}
}
