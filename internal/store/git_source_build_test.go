package store

import (
	"context"
	"errors"
	"testing"
)

func TestGitSourceBuildSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "railpack"}); err != nil {
		t.Fatalf("SaveGitSource() error = %v", err)
	}
	if err := db.SetGitSourceBuild(ctx, "web", "dockerfile", "apps/api/deploy/Dockerfile", "apps/api"); err != nil {
		t.Fatalf("SetGitSourceBuild() error = %v", err)
	}
	got, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.BuildType != "dockerfile" || got.BuildPath != "apps/api/deploy/Dockerfile" || got.BaseDirectory != "apps/api" {
		t.Fatalf("got %+v", got)
	}
	b, err := got.SpecBuild()
	if err != nil {
		t.Fatal(err)
	}
	if b.BaseDirectory != "apps/api" || b.Path != "deploy/Dockerfile" {
		t.Fatalf("SpecBuild() = %+v", b)
	}
	if err := db.SetGitSourceBuild(ctx, "missing", "dockerfile", "", ""); !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("missing source error = %v", err)
	}
}

func TestGitSourceSpecBuildRejectsEscapingBase(t *testing.T) {
	g := GitSource{ServiceName: "web", BuildType: "dockerfile", BaseDirectory: "../x"}
	if _, err := g.SpecBuild(); err == nil {
		t.Fatal("want an error for a base directory that escapes the repository")
	}
}
