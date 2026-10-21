package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func gitConnServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	conns := []apiclient.GitProviderConnectionResource{
		{ID: "gc_1", Provider: "gitlab", Label: "ann @ gitlab.com", AccountName: "ann", Host: "gitlab.com", IsDefault: true, Authorized: true, SourceCount: 2},
		{ID: "gc_2", Provider: "gitlab", Label: "Work", AccountName: "bob", Host: "git.corp.example", AuthorizePath: "/api/v1/gitlab-app/connect?connection_id=gc_2"},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/git-connections":
			_ = json.NewEncoder(w).Encode(conns)
		case r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(apiclient.GitProviderConnectionResource{ID: "gc_2", Label: "Renamed"})
		case r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(apiclient.GitDisconnectResource{Disconnected: true, Note: "The token was revoked.", AffectedSources: []string{"web"}})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.GitRepoListResource{Items: []apiclient.GitRepoResource{{FullName: "ann/web", Owner: "ann", Account: "ann"}}, Total: 1, Loaded: 1})
		}
	}))
}

func TestRun_GitProvidersConnections_ListsEveryAccount(t *testing.T) {
	var seen []string
	srv := gitConnServer(t, &seen)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "connections", "--provider", "gitlab", "--api-url", srv.URL})
	for _, want := range []string{"gc_1", "gc_2", "ann @ gitlab.com", "git.corp.example", "not authorized", "connection_id=gc_2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if len(seen) != 1 || seen[0] != "GET /api/v1/git-connections?provider=gitlab" {
		t.Errorf("requests = %v", seen)
	}
}

func TestRun_GitProvidersRenameAndDisconnect(t *testing.T) {
	var seen []string
	srv := gitConnServer(t, &seen)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "rename", "gc_2", "Renamed", "--api-url", srv.URL})
	if !strings.Contains(stdout, "renamed to Renamed") {
		t.Errorf("rename stdout = %q", stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"git-providers", "disconnect", "--id", "gc_2", "--api-url", srv.URL})
	if !strings.Contains(stdout, "needs reconnect: web") {
		t.Errorf("disconnect stdout = %q, want the dependent source named", stdout)
	}
	if len(seen) != 2 || seen[0] != "PATCH /api/v1/git-connections/gc_2" || seen[1] != "DELETE /api/v1/git-connections/gc_2" {
		t.Errorf("requests = %v", seen)
	}
}

func TestRun_GitProvidersDisconnect_RequiresID(t *testing.T) {
	var outBuf, errBuf strings.Builder
	got := run("levelrail-cli-test", []string{"git-providers", "disconnect", "--api-url", "http://127.0.0.1:1"}, &outBuf, &errBuf, envMap())
	if got != exitUsage || !strings.Contains(errBuf.String(), "requires --id") {
		t.Errorf("exit = %d, stderr = %q", got, errBuf.String())
	}
}

func TestRun_GitProvidersRepos_ConnectionFilterAndAccountColumn(t *testing.T) {
	var seen []string
	srv := gitConnServer(t, &seen)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "repos", "gitlab", "--connection", "gc_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "ACCOUNT") || !strings.Contains(stdout, "ann/web") {
		t.Errorf("stdout = %q", stdout)
	}
	if len(seen) != 1 || !strings.Contains(seen[0], "connection=gc_1") || !strings.Contains(seen[0], "provider=gitlab") {
		t.Errorf("requests = %v", seen)
	}
}
