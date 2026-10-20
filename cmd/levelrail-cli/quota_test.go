package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_Quota(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reply := apiclient.QuotaResource{
		Scope: "project", ScopeID: "proj_1", Name: "shop",
		Effective: apiclient.QuotaLimits{MaxApps: 5},
		Usage:     apiclient.QuotaUsage{Apps: 4},
	}
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantBody   string
	}{
		{"show", []string{"quota", "show", "project", "proj_1"}, "GET", "/api/v1/projects/proj_1/quota", ""},
		{"set", []string{"quota", "set", "team", "org_a", "--max-apps", "5", "--max-cpu", "1.5"}, "PUT", "/api/v1/teams/org_a/quota", `"max_apps":5`},
		{"clear", []string{"quota", "clear", "team", "org_a"}, "DELETE", "/api/v1/teams/org_a/quota", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := teamsServer(t, reply)
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli-test", append(tc.args, "--api-url", srv.URL, "--token", "tok"), &stdout, &stderr, envMap())
			if code != exitOK {
				t.Fatalf("exit = %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
			}
			r := (*got)[0]
			if r.method != tc.wantMethod || r.path != tc.wantPath || !strings.Contains(r.body, tc.wantBody) {
				t.Errorf("request = %s %s %s", r.method, r.path, r.body)
			}
			if !strings.Contains(stdout.String(), "4") || !strings.Contains(stdout.String(), "shop") {
				t.Errorf("output = %q, want usage and name", stdout.String())
			}
		})
	}
}

func TestRun_QuotaRejectsBadScope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", []string{"quota", "show", "org", "x", "--api-url", "http://127.0.0.1:1", "--token", "tok"}, &stdout, &stderr, envMap())
	if code != exitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
}
