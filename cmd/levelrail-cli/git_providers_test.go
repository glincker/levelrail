package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRun_GitProviders(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []gitProviderResource{
		{Provider: "github", Connected: true, CanListBranches: true, CanRegisterWebhook: true, CanAuthClone: true},
		{Provider: "gitlab", Connected: false},
		{Provider: "bitbucket", Connected: false},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "--api-url", srv.URL})

	if gotPath != "/api/v1/git-providers" {
		t.Errorf("path = %s, want /api/v1/git-providers", gotPath)
	}
	if !strings.Contains(stdout, "github") || !strings.Contains(stdout, "gitlab") || !strings.Contains(stdout, "bitbucket") {
		t.Errorf("stdout = %q, want all three providers listed", stdout)
	}
}

func TestRun_GitProviders_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []gitProviderResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"git-providers", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no providers") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_GitProviders_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"git-providers", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_GitProviders_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"git-providers", "-h"})
	if !strings.Contains(stderr, "git-providers") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
