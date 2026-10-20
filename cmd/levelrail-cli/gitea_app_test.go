package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_GiteaApp_Status(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, giteaAppStatusResource{Connected: true, Authorized: true, InstanceURL: "https://git.example.com", ClientID: "cid"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "status", "--api-url", srv.URL})

	if gotPath != "/api/v1/gitea-app" {
		t.Errorf("path = %s, want /api/v1/gitea-app", gotPath)
	}
	if !strings.Contains(stdout, "connected: true") || !strings.Contains(stdout, "authorized:   true") {
		t.Errorf("stdout = %q, want connected/authorized lines", stdout)
	}
}

func TestRun_GiteaApp_Disconnect(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "disconnect", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/gitea-app" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/gitea-app", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"disconnected": true`) && !strings.Contains(stdout, "gitea app disconnected") {
		t.Errorf("stdout = %q, want a disconnect confirmation", stdout)
	}
}

func TestRun_GiteaApp_Repos(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []giteaAppRepoResource{{FullName: "acme/widgets", DefaultBranch: "main"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "repos", "--api-url", srv.URL})

	if gotPath != "/api/v1/gitea-app/repos" {
		t.Errorf("path = %s, want /api/v1/gitea-app/repos", gotPath)
	}
	if !strings.Contains(stdout, "acme/widgets") {
		t.Errorf("stdout = %q, want acme/widgets listed", stdout)
	}
}

func TestRun_GiteaApp_Branches(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitAppBranchResource{{Name: "main"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "branches", "acme", "widgets", "--api-url", srv.URL})

	if gotPath != "/api/v1/gitea-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/gitea-app/repos/acme/widgets/branches", gotPath)
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want main listed", stdout)
	}
}

func TestRun_GiteaApp_Branches_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"gitea-app", "branches", "acme", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an owner and a repo") {
		t.Errorf("stderr = %q, want a missing-arg usage error", stderr.String())
	}
}

func TestRun_GiteaApp_UseAsSource(t *testing.T) {
	var gotMethod, gotPath string
	srv := newEchoServer(t, &gotMethod, &gotPath, gitSourceResource{ServiceName: "web"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "use-as-source", "acme", "widgets", "--app-name", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/gitea-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/gitea-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "web") {
		t.Errorf("stdout = %q, want web reflected", stdout)
	}
}

func TestRun_GiteaApp_UseAsSource_MissingAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"gitea-app", "use-as-source", "acme", "widgets", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --app-name") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_GiteaApp_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"gitea-app", "-h"})
	if !strings.Contains(stdout, "gitea-app repos") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_GiteaApp_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"no gitea app is connected"}`)

	stderr := runCLIExpectAPIError(t, []string{"gitea-app", "repos", "--api-url", srv.URL})
	if !strings.Contains(stderr, "no gitea app is connected") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}
