package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rolesCall struct {
	Method string
	Path   string
	Body   string
}

// newRolesServer answers GET /api/v1/roles with the fixed list and records every call.
func newRolesServer(t *testing.T, calls *[]rolesCall) *httptest.Server {
	t.Helper()
	roles := []roleResource{
		{ID: "role_viewer", Name: "viewer", Abilities: []string{"read"}, Visibility: "all", Builtin: true, UserCount: 2},
		{ID: "role_guest", Name: "guest", Abilities: []string{"read"}, Visibility: "granted", Builtin: true},
		{ID: "role_qa", Name: "qa", Description: "d", Abilities: []string{"read", "deploy"}, Visibility: "all"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*calls = append(*calls, rolesCall{r.Method, r.URL.Path, string(b)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/roles":
			_ = json.NewEncoder(w).Encode(roles)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/environment-grants"):
			_ = json.NewEncoder(w).Encode(map[string]any{"environment_ids": []string{"env_dev"}})
		case strings.HasSuffix(r.URL.Path, "/role"):
			_ = json.NewEncoder(w).Encode(userResource{ID: "user_1", Email: "g@example.com", Role: "guest"})
		default:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(roleResource{ID: "role_new", Name: "new"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_RolesList(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	stdout, _ := runCLIExpectOK(t, []string{"roles", "list", "--api-url", srv.URL})
	for _, want := range []string{"role_guest", "granted", "qa", "read,deploy"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestRun_RolesCreate(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	runCLIExpectOK(t, []string{"roles", "create", "tourist", "--abilities", "read", "--visibility", "granted", "--description", "x", "--api-url", srv.URL})
	last := calls[len(calls)-1]
	if last.Method != http.MethodPost || last.Path != "/api/v1/roles" {
		t.Fatalf("call = %+v", last)
	}
	var body struct {
		Name       string   `json:"name"`
		Abilities  []string `json:"abilities"`
		Visibility string   `json:"visibility"`
	}
	_ = json.Unmarshal([]byte(last.Body), &body)
	if body.Name != "tourist" || body.Visibility != "granted" || len(body.Abilities) != 1 {
		t.Errorf("body = %+v", body)
	}
	if got := runCLIExpectValidationError(t, []string{"roles", "create", "x", "--api-url", srv.URL}); !strings.Contains(got, "--abilities") {
		t.Errorf("stderr = %q, want a missing --abilities error", got)
	}
}

func TestRun_RolesUpdateMergesOnlyPassedFlags(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	runCLIExpectOK(t, []string{"roles", "update", "qa", "--abilities", "read", "--api-url", srv.URL})
	last := calls[len(calls)-1]
	if last.Method != http.MethodPut || last.Path != "/api/v1/roles/role_qa" {
		t.Fatalf("call = %+v", last)
	}
	if !strings.Contains(last.Body, `"name":"qa"`) || !strings.Contains(last.Body, `"description":"d"`) || strings.Contains(last.Body, "deploy") {
		t.Errorf("body = %s, want name and description kept, abilities replaced", last.Body)
	}
}

func TestRun_RolesDeleteAndUnknown(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	runCLIExpectOK(t, []string{"roles", "delete", "qa", "--api-url", srv.URL})
	last := calls[len(calls)-1]
	if last.Method != http.MethodDelete || last.Path != "/api/v1/roles/role_qa" {
		t.Errorf("call = %+v", last)
	}
	if got := runCLIExpectValidationError(t, []string{"roles", "delete", "nope", "--api-url", srv.URL}); !strings.Contains(got, "no role") {
		t.Errorf("stderr = %q", got)
	}
}

func TestRun_UsersRoleSet(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	stdout, _ := runCLIExpectOK(t, []string{"users", "role", "set", "user_1", "guest", "--api-url", srv.URL})
	last := calls[len(calls)-1]
	if last.Method != http.MethodPut || last.Path != "/api/v1/users/user_1/role" || !strings.Contains(last.Body, `"role_id":"role_guest"`) {
		t.Errorf("call = %+v", last)
	}
	if !strings.Contains(stdout, "guest") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_UsersGrants(t *testing.T) {
	var calls []rolesCall
	srv := newRolesServer(t, &calls)
	runCLIExpectOK(t, []string{"users", "grants", "set", "user_1", "--environment", "env_dev", "--environment", "env_test", "--api-url", srv.URL})
	last := calls[len(calls)-1]
	if last.Method != http.MethodPut || last.Path != "/api/v1/users/user_1/environment-grants" || !strings.Contains(last.Body, `["env_dev","env_test"]`) {
		t.Errorf("set call = %+v", last)
	}
	runCLIExpectOK(t, []string{"users", "grants", "set", "user_1", "--api-url", srv.URL})
	if last := calls[len(calls)-1]; !strings.Contains(last.Body, `"environment_ids":[]`) {
		t.Errorf("clear body = %s, want an empty list", last.Body)
	}
	stdout, _ := runCLIExpectOK(t, []string{"users", "grants", "get", "user_1", "--api-url", srv.URL})
	if last := calls[len(calls)-1]; last.Method != http.MethodGet || !strings.Contains(stdout, "env_dev") {
		t.Errorf("get call = %+v stdout = %q", last, stdout)
	}
}
