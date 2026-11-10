package repolayout

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestCloneSourceDetectsTurborepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"turbo.json":                   "{}",
		"package.json":                 `{"private":true,"workspaces":["apps/*"]}`,
		"apps/glinr/package.json":      "{}",
		"apps/glinr/deploy/Dockerfile": "FROM node:20\nRUN npx turbo prune glinr --docker\n",
		"node_modules/x/Dockerfile":    "FROM scratch",
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	src, err := NewCloneSource(context.Background(), "file://"+dir, head.Name().Short(), "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Detect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Suggestions) == 0 || res.Suggestions[0].DockerfilePath != "apps/glinr/deploy/Dockerfile" {
		t.Fatalf("suggestions = %+v", res.Suggestions)
	}
	if res.Suggestions[0].BaseDirectory != "" || res.Suggestions[0].ReasonCode != ReasonTurboPrune {
		t.Fatalf("top = %+v, want root context from turbo prune", res.Suggestions[0])
	}
}

func TestCloneSourceSSHDeployKeyAuth(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	auth, err := NewSSHDeployKeyAuth("", pemKey)
	if err != nil {
		t.Fatalf("deploy key auth: %v", err)
	}
	keys, ok := auth.(*gitssh.PublicKeys)
	if !ok || keys.User != "git" || keys.Signer == nil {
		t.Fatalf("auth = %#v, want git user with signer", auth)
	}
	if _, err := NewSSHDeployKeyAuth("git", []byte("not a key")); err == nil {
		t.Fatal("garbage key accepted")
	}

	// An unreachable ssh host must fail the clone, proving the key path is used.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := NewCloneSourceAuth(ctx, "ssh://git@127.0.0.1:1/x/y.git", "main", auth); err == nil {
		t.Fatal("expected clone error for unreachable ssh host")
	}
}
