package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestInstallGitNetguardBlocksInternalHosts(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	t.Setenv(gitAllowPrivateEnv, "false")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	installGitNetguard(nil)
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
	installGitNetguard(nil)
	_, _ = git.CloneContext(context.Background(), memory.NewStorage(), nil, &git.CloneOptions{URL: srv.URL + "/r.git"})
	if !hit {
		t.Error("the loopback server was not contacted although the git override is on")
	}
}

func TestGitGuardReadsSharedSettingsWithoutRestart(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	noEnv := func(string) (string, bool) { return "", false }
	boot := runtimesettings.New(db, noEnv)
	shared := runtimesettings.New(db, noEnv)
	installGitNetguard(boot)
	t.Cleanup(func() { installGitNetguard(nil) })
	if got := gitSSHAllowList(); len(got) != 0 {
		t.Fatalf("allowlist before save = %v, want empty", got)
	}
	useSharedGitSettings(shared)
	if _, err := shared.Set(ctx, runtimesettings.KeyGitSSHScanAllowInternal, json.RawMessage(`["10.20.0.0/16"]`), "root"); err != nil {
		t.Fatal(err)
	}
	if got := gitSSHAllowList(); !slices.Contains(got, "10.20.0.0/16") {
		t.Fatalf("allowlist after a save through the shared service = %v, want 10.20.0.0/16", got)
	}
}
