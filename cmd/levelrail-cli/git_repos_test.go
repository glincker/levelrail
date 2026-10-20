package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_GitProvidersRepos_PassesFiltersAndPages(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/repos" {
			t.Errorf("path = %s", r.URL.Path)
		}
		queries = append(queries, r.URL.RawQuery)
		res := apiclient.GitRepoListResource{Provider: "github", Total: 3, Loaded: 5, Cap: 10000}
		if r.URL.Query().Get("offset") == "" {
			next := 2
			res.Items = []apiclient.GitRepoResource{{FullName: "acme/a", Owner: "acme"}, {FullName: "acme/b", Owner: "acme", Archived: true}}
			res.HasMore, res.NextOffset = true, &next
		} else {
			res.Items = []apiclient.GitRepoResource{{FullName: "acme/c", Owner: "acme", Fork: true}}
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "repos", "github", "--api-url", srv.URL, "--search", "acme", "--archived", "exclude", "--refresh", "--all"})
	for _, want := range []string{"acme/a", "acme/b", "acme/c", "showing 3 of 3 matches"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if len(queries) != 2 || !strings.Contains(queries[0], "refresh=1") || strings.Contains(queries[1], "refresh") {
		t.Errorf("queries = %v, want refresh only on the first page", queries)
	}
	for _, want := range []string{"provider=github", "search=acme", "archived=exclude", "limit=500"} {
		if !strings.Contains(queries[0], want) {
			t.Errorf("first query %q missing %q", queries[0], want)
		}
	}
}

func TestRun_GitProvidersRepos_SurfacesTruncationAndMoreHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		next := 100
		_ = json.NewEncoder(w).Encode(apiclient.GitRepoListResource{
			Provider: "gitlab", Items: []apiclient.GitRepoResource{{FullName: "g/p"}}, Total: 4000, Loaded: 4000,
			HasMore: true, NextOffset: &next, Truncated: true, Cap: 4000,
		})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "repos", "gitlab", "--api-url", srv.URL})
	if !strings.Contains(stdout, "more with --offset 100") || !strings.Contains(stdout, "more than the 4000 repositories") {
		t.Errorf("stdout = %q, want the more-results hint and the truncation warning", stdout)
	}
}
