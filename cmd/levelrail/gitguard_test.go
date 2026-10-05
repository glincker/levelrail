package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

func TestInstallGitNetguardBlocksInternalHosts(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	t.Setenv(gitAllowPrivateEnv, "false")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	installGitNetguard()
	for _, u := range []string{srv.URL + "/r.git", "http://169.254.169.254/latest/meta-data/"} {
		_, err := git.CloneContext(context.Background(), memory.NewStorage(), nil, &git.CloneOptions{URL: u})
		if err == nil || !strings.Contains(err.Error(), "internal") {
			t.Errorf("clone %s: err = %v, want a netguard refusal", u, err)
		}
	}
	if hit {
		t.Error("the loopback server was contacted")
	}
}

func TestInstallGitNetguardGitOverrideIsIndependent(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	t.Setenv(gitAllowPrivateEnv, "true")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	installGitNetguard()
	_, _ = git.CloneContext(context.Background(), memory.NewStorage(), nil, &git.CloneOptions{URL: srv.URL + "/r.git"})
	if !hit {
		t.Error("the loopback server was not contacted although the git override is on")
	}
}
