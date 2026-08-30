package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_UsersList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []userResource{{ID: "user_1", Email: "a@example.com", Role: "operator"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/users" {
		t.Errorf("path = %q, want /api/v1/users", gotPath)
	}
	if !strings.Contains(stdout, "a@example.com") || !strings.Contains(stdout, "operator") {
		t.Errorf("stdout = %q, want the user's email and role", stdout)
	}
}

func TestRun_UsersList_JSON(t *testing.T) {
	srv := newListEchoServer(t, nil, []userResource{{ID: "user_1", Email: "a@example.com"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "list", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"id": "user_1"`) {
		t.Errorf("stdout = %q, want the user as JSON", stdout)
	}
}

func TestRun_UsersCreate_WithRole(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createUserRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(userResource{ID: "user_1", Email: gotBody.Email, Role: gotBody.Role})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"users", "create",
		"--email", "teammate@example.com", "--password", "a-real-password", "--role", "operator",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/users" {
		t.Errorf("request = %s %s, want POST /api/v1/auth/users", gotMethod, gotPath)
	}
	if gotBody.Role != "operator" || len(gotBody.Abilities) != 0 {
		t.Errorf("request body = %+v, want role=operator, no abilities", gotBody)
	}
	if !strings.Contains(stdout, "teammate@example.com") {
		t.Errorf("stdout = %q, want a created confirmation", stdout)
	}
}

func TestRun_UsersCreate_WithAbilities(t *testing.T) {
	var gotBody createUserRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(userResource{ID: "user_1", Email: gotBody.Email, Abilities: gotBody.Abilities})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"users", "create",
		"--email", "teammate@example.com", "--password", "a-real-password", "--abilities", "read,deploy",
		"--api-url", srv.URL,
	})
	if gotBody.Role != "" || len(gotBody.Abilities) != 2 {
		t.Errorf("request body = %+v, want no role, abilities [read deploy]", gotBody)
	}
}

func TestRun_UsersCreate_RoleAndAbilitiesMutuallyExclusive(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{
		"users", "create", "--email", "a@example.com", "--password", "a-real-password",
		"--role", "operator", "--abilities", "read",
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderrBuf.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want a mutual-exclusivity error", stderrBuf.String())
	}
}

func TestRun_UsersCreate_MissingRoleAndAbilities(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{
		"users", "create", "--email", "a@example.com", "--password", "a-real-password",
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderrBuf.String(), "one of --role or --abilities") {
		t.Errorf("stderr = %q, want a missing role/abilities error", stderrBuf.String())
	}
}

func TestRun_UsersCreate_MissingEmail(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{
		"users", "create", "--password", "a-real-password", "--role", "viewer",
	}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderrBuf.String(), "--email is required") {
		t.Errorf("stderr = %q, want a missing-email error", stderrBuf.String())
	}
}

func TestRun_UsersSetAbilities_WithRole(t *testing.T) {
	var gotPath string
	var gotBody updateUserAbilitiesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userResource{ID: "user_1", Email: "other@example.com", Role: gotBody.Role})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "set-abilities", "user_1", "--role", "viewer", "--api-url", srv.URL})
	if gotPath != "/api/v1/users/user_1/abilities" {
		t.Errorf("path = %q, want /api/v1/users/user_1/abilities", gotPath)
	}
	if gotBody.Role != "viewer" {
		t.Errorf("request body Role = %q, want %q", gotBody.Role, "viewer")
	}
	if !strings.Contains(stdout, "other@example.com") {
		t.Errorf("stdout = %q, want a confirmation", stdout)
	}
}

func TestRun_UsersSetAbilities_NoID(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{"users", "set-abilities", "--role", "viewer"}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderrBuf.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderrBuf.String())
	}
}

func TestRun_UsersDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "delete", "user_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/users/user_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/users/user_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `user "user_1" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_UsersDelete_SelfLockoutError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"cannot remove your own account"}`)

	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{"users", "delete", "user_1", "--api-url", srv.URL}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d", got, exitAPIError)
	}
	if !strings.Contains(stderrBuf.String(), "cannot remove your own account") {
		t.Errorf("stderr = %q, want the server's self-lockout message", stderrBuf.String())
	}
}

func TestRun_UsersRoles(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []roleResource{
		{Name: "admin", Description: "full access", Abilities: []string{"root"}},
		{Name: "operator", Description: "deploy and manage", Abilities: []string{"read", "read:sensitive", "write", "deploy"}},
		{Name: "viewer", Description: "read only", Abilities: []string{"read"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"users", "roles", "--api-url", srv.URL})
	if gotPath != "/api/v1/roles" {
		t.Errorf("path = %q, want /api/v1/roles", gotPath)
	}
	for _, want := range []string{"admin", "operator", "viewer"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to list role %q", stdout, want)
		}
	}
}

func TestRun_Users_UnknownSubcommand(t *testing.T) {
	var stdoutBuf, stderrBuf strings.Builder
	got := run("levelrail-cli-test", []string{"users", "bogus"}, &stdoutBuf, &stderrBuf, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderrBuf.String(), "unknown users subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderrBuf.String())
	}
}

func TestRun_Users_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"users", "-h"})
	if !strings.Contains(stdout, "users list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
