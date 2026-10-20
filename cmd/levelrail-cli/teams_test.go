package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type recordedRequest struct {
	method, path, body, team, project string
}

func teamsServer(t *testing.T, reply any) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var got []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, recordedRequest{r.Method, r.URL.Path, string(b), r.Header.Get(apiclient.ScopeTeamHeader), r.Header.Get(apiclient.ScopeProjectHeader)})
		w.Header().Set("Content-Type", "application/json")
		if reply == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestRun_Teams(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name       string
		args       []string
		reply      any
		wantMethod string
		wantPath   string
		wantBody   string
		wantOut    string
	}{
		{"list", []string{"teams", "list"}, []apiclient.TeamResource{{ID: "org_a", Name: "Acme", Role: "owner", MemberCount: 2}}, "GET", "/api/v1/teams", "", "Acme"},
		{"create", []string{"teams", "create", "Acme"}, apiclient.TeamResource{ID: "org_a", Name: "Acme", Role: "owner"}, "POST", "/api/v1/teams", `"name":"Acme"`, "org_a"},
		{"members list", []string{"members", "list", "org_a"}, []apiclient.TeamMemberResource{{UserID: "u1", Email: "a@x.test", Role: "admin"}}, "GET", "/api/v1/teams/org_a/members", "", "a@x.test"},
		{"set role", []string{"teams", "members", "set-role", "org_a", "u1", "viewer"}, nil, "PUT", "/api/v1/teams/org_a/members/u1", `"role":"viewer"`, "u1 is now viewer"},
		{"leave", []string{"members", "leave", "org_a"}, nil, "DELETE", "/api/v1/teams/org_a/members/me", "", "removed"},
		{"invite link", []string{"teams", "invites", "create", "org_a", "--role", "developer"}, apiclient.TeamInviteResource{ID: "tinv_1", Role: "developer", Status: "pending", Token: "secret-1"}, "POST", "/api/v1/teams/org_a/invites", `"role":"developer"`, "secret-1"},
		{"transfer", []string{"teams", "transfer", "org_a", "u2"}, nil, "POST", "/api/v1/teams/org_a/transfer", `"user_id":"u2"`, "transferred"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := teamsServer(t, tc.reply)
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli-test", append(tc.args, "--api-url", srv.URL, "--token", "tok"), &stdout, &stderr, envMap())
			if code != exitOK {
				t.Fatalf("exit = %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
			}
			if len(*got) != 1 {
				t.Fatalf("requests = %d, want 1", len(*got))
			}
			r := (*got)[0]
			if r.method != tc.wantMethod || r.path != tc.wantPath || !strings.Contains(r.body, tc.wantBody) {
				t.Errorf("request = %s %s %s, want %s %s with %s", r.method, r.path, r.body, tc.wantMethod, tc.wantPath, tc.wantBody)
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantOut)
			}
		})
	}
}

func TestRun_ScopeFlagsSendHeaders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, got := teamsServer(t, []apiclient.TeamResource{})
	var stdout, stderr bytes.Buffer
	args := []string{"--project", "proj_1", "teams", "list", "--team", "org_a", "--api-url", srv.URL, "--token", "tok"}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if r := (*got)[0]; r.team != "org_a" || r.project != "proj_1" {
		t.Errorf("scope headers = %q %q, want org_a proj_1", r.team, r.project)
	}
}

func TestRun_TeamsUseSavesContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, got := teamsServer(t, apiclient.TeamResource{ID: "org_a", Name: "Acme"})
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"teams", "use", "org_a", "--project", "proj_9", "--api-url", srv.URL, "--token", "tok"}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	saved, err := apiclient.ReadContext("levelrail-cli-test", "default")
	if err != nil || saved.Team != "org_a" || saved.Project != "proj_9" {
		t.Fatalf("saved context = %+v, %v", saved, err)
	}
	stdout.Reset()
	if code := run("levelrail-cli-test", []string{"teams", "show", "org_a", "--api-url", srv.URL, "--token", "tok"}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("show exit = %d stderr=%q", code, stderr.String())
	}
	if last := (*got)[len(*got)-1]; last.team != "org_a" || last.project != "proj_9" {
		t.Errorf("saved context not applied: %+v", last)
	}
}

func TestExtractScopeFlags(t *testing.T) {
	tests := []struct {
		in        []string
		wantArgs  string
		wantScope apiclient.Scope
	}{
		{[]string{"apps", "list"}, "apps list", apiclient.Scope{}},
		{[]string{"--team=org_a", "apps", "list"}, "apps list", apiclient.Scope{Team: "org_a"}},
		{[]string{"--project", "p1", "apps", "list", "--team", "t1"}, "apps list", apiclient.Scope{Team: "t1", Project: "p1"}},
		{[]string{"apply", "--project", "p1"}, "apply --project p1", apiclient.Scope{}},
	}
	for _, tc := range tests {
		args, scope := extractScopeFlags(tc.in)
		if strings.Join(args, " ") != tc.wantArgs || scope != tc.wantScope {
			t.Errorf("extractScopeFlags(%v) = %v %+v, want %q %+v", tc.in, args, scope, tc.wantArgs, tc.wantScope)
		}
	}
}
