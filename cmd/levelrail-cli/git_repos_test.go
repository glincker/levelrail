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

func TestRun_GitProviders_ShowsHealthAndReconnectHint(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitProviderResource{
		{Provider: "github", Connected: true, Connection: apiclient.GitConnectionResource{Status: "ok"}},
		{Provider: "gitlab", Connected: true, Connection: apiclient.GitConnectionResource{Status: "needs_reauthorize", Detail: "the authorization was revoked or has expired"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "--api-url", srv.URL})
	for _, want := range []string{"HEALTH", "needs_reauthorize", "Press Reconnect", "/api/v1/gitlab-app/connect"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "github: ") {
		t.Errorf("a healthy provider must not print a reconnect hint:\n%s", stdout)
	}
}

func TestRun_AppsGitSourceRegisterWebhook(t *testing.T) {
	respond := func(state, msg string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/git-source/webhook/register" {
				t.Errorf("%s %s", r.Method, r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(apiclient.GitSourceResource{
				ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", TriggerMode: "push",
				WebhookURLWarning:   "private_address",
				WebhookRegistration: &apiclient.GitWebhookRegistrationResource{State: state, Error: msg},
				Poll:                &apiclient.GitPollStatusResource{Enabled: true},
			})
		}))
	}

	ok := respond("registered", "")
	defer ok.Close()
	stdout, _ := runCLIExpectOK(t, []string{"apps", "git-source", "register-webhook", "web", "--api-url", ok.URL})
	for _, want := range []string{"webhook:        registered", "webhook address is private or local", "poll:           not checked yet"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}

	failed := respond("failed", "permission denied")
	defer failed.Close()
	var outBuf, errBuf strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "git-source", "register-webhook", "web", "--api-url", failed.URL}, &outBuf, &errBuf, envMap())
	if got != exitCheckFailed || !strings.Contains(outBuf.String(), "failed (permission denied)") {
		t.Errorf("exit = %d, stdout = %q; want a non-zero exit with the error shown", got, outBuf.String())
	}
}
