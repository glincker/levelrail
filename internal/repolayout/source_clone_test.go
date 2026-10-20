package repolayout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
