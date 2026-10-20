package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_GitHubApp_Status(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, gitHubAppStatusResource{Connected: true, Installed: true, AccountLogin: "acme"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "status", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app" {
		t.Errorf("path = %s, want /api/v1/github-app", gotPath)
	}
	if !strings.Contains(stdout, "connected: true") || !strings.Contains(stdout, "account_login:       acme") {
		t.Errorf("stdout = %q, want connected/account_login lines", stdout)
	}
}

func TestRun_GitHubApp_Status_NotConnected(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppStatusResource{Connected: false})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "connected: false") {
		t.Errorf("stdout = %q, want connected: false", stdout)
	}
	if strings.Contains(stdout, "installed") {
		t.Errorf("stdout = %q, want no installed line when not connected", stdout)
	}
}

func TestRun_GitHubApp_Disconnect(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "disconnect", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/github-app" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/github-app", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"disconnected": true`) && !strings.Contains(stdout, "github app disconnected") {
		t.Errorf("stdout = %q, want a disconnect confirmation", stdout)
	}
}

func TestRun_GitHubApp_Disconnect_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"no github app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"github-app", "disconnect", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no github app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_GitHubApp_Repos(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, gitHubAppRepoListResource{
		Repos: []gitHubAppRepoResource{
			{FullName: "acme/widgets", DefaultBranch: "main", AccountType: "organization"},
		},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "repos", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app/repos" {
		t.Errorf("path = %s, want /api/v1/github-app/repos", gotPath)
	}
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
}

func TestRun_GitHubApp_Repos_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppRepoListResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no repositories") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_GitHubApp_Repos_PartialErrors(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppRepoListResource{
		Repos: []gitHubAppRepoResource{{FullName: "acme/widgets", DefaultBranch: "main", AccountType: "organization"}},
		Errors: []gitHubAppRepoListErrResource{
			{AccountLogin: "suspended-org", Error: "installation suspended"},
		},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
	if !strings.Contains(stdout, "suspended-org") || !strings.Contains(stdout, "installation suspended") {
		t.Errorf("stdout = %q, want the suspended-org warning", stdout)
	}
}

func TestRun_GitHubApp_Repos_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"no github app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"github-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no github app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_GitHubApp_Branches(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitAppBranchResource{{Name: "main", CommitSHA: "abc123"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "branches", "acme", "widgets", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/github-app/repos/acme/widgets/branches", gotPath)
	}
	if !strings.Contains(stdout, "main") || !strings.Contains(stdout, "abc123") {
		t.Errorf("stdout = %q, want main/abc123 listed", stdout)
	}
}

func TestRun_GitHubApp_Branches_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "branches", "acme", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an owner and a repo") {
		t.Errorf("stderr = %q, want a missing-arg usage error", stderr.String())
	}
}

func TestRun_GitHubApp_UseAsSource(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody useRepoAsSourceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(useGitHubRepoAsSourceResponse{
			GitSourceResource: gitSourceResource{ServiceName: gotBody.AppName, RepoURL: "https://github.com/acme/widgets.git"},
			WebhookRegistered: true,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "use-as-source", "acme", "widgets", "--app-name", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/github-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/github-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if gotBody.AppName != "web" {
		t.Errorf("request body = %+v, want AppName=web", gotBody)
	}
	if !strings.Contains(stdout, "webhook_registered: true") {
		t.Errorf("stdout = %q, want webhook_registered: true", stdout)
	}
}

func TestRun_GitHubApp_UseAsSource_MissingAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "use-as-source", "acme", "widgets", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --app-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_GitHubApp_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"github-app", "-h"})
	if !strings.Contains(stdout, "github-app repos") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_GitHubApp_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown github-app subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_GitHubApp_Installations_List(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, gitHubAppInstallationListResource{
		Installations: []gitHubAppInstallationResource{
			{ID: 1, InstallationID: 42, AccountLogin: "acme", AccountType: "organization", ConnectedAt: "2026-01-01T00:00:00Z"},
		},
		AddOrgURL: "https://github.com/apps/levelrail/installations/new",
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "list", "--api-url", srv.URL})

	if gotPath != "/api/v1/github-app/installations" {
		t.Errorf("path = %s, want /api/v1/github-app/installations", gotPath)
	}
	if !strings.Contains(stdout, "acme") || !strings.Contains(stdout, "organization") {
		t.Errorf("stdout = %q, want the acme installation listed", stdout)
	}
}

func TestRun_GitHubApp_Installations_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppInstallationListResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no connected accounts") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_GitHubApp_Installations_Add(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppInstallationListResource{
		AddOrgURL: "https://github.com/apps/levelrail/installations/new",
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "add", "--api-url", srv.URL})
	if !strings.Contains(stdout, "https://github.com/apps/levelrail/installations/new") {
		t.Errorf("stdout = %q, want the add-org URL", stdout)
	}
}

func TestRun_GitHubApp_Installations_Add_NoURL(t *testing.T) {
	srv := newListEchoServer(t, nil, gitHubAppInstallationListResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "add", "--api-url", srv.URL})
	if !strings.Contains(stdout, "reconnect the GitHub App") {
		t.Errorf("stdout = %q, want the reconnect-first explanation", stdout)
	}
}

func TestRun_GitHubApp_Installations_Remove(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "remove", "7", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/github-app/installations/7" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/github-app/installations/7", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"removed": true`) && !strings.Contains(stdout, "installation 7 removed") {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_GitHubApp_Installations_Remove_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"still in use by: web; disconnect or move those git sources first"}`)

	stderr := runCLIExpectAPIError(t, []string{"github-app", "installations", "remove", "7", "--api-url", srv.URL})
	if !strings.Contains(stderr, "still in use by: web") {
		t.Errorf("stderr = %q, want the server's error verbatim", stderr)
	}
}

func TestRun_GitHubApp_Installations_Remove_InvalidID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "installations", "remove", "not-a-number", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "not a valid id") {
		t.Errorf("stderr = %q, want an invalid-id usage error", stderr.String())
	}
}

func TestRun_GitHubApp_Installations_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"github-app", "installations", "-h"})
	if !strings.Contains(stdout, "github-app installations list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_GitHubApp_Installations_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "installations", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown github-app installations subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
