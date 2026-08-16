package store

import (
	"context"
	"errors"
	"testing"
)

func TestSaveAndGetGitSource(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := GitSource{
		ServiceName: "web",
		RepoURL:     "https://github.com/org/web.git",
		Branch:      "main",
		BuildType:   "dockerfile",
		BuildPath:   "./Dockerfile",
	}
	if err := db.SaveGitSource(ctx, want); err != nil {
		t.Fatalf("SaveGitSource() error = %v", err)
	}

	got, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if got.ServiceName != want.ServiceName || got.RepoURL != want.RepoURL || got.Branch != want.Branch ||
		got.BuildType != want.BuildType || got.BuildPath != want.BuildPath {
		t.Errorf("GetGitSource() = %+v, want fields to match %+v", got, want)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("GetGitSource() timestamps not populated: %+v", got)
	}
}

func TestGetGitSource_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetGitSource(ctx, "ghost")
	if !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("GetGitSource() error = %v, want ErrGitSourceNotFound", err)
	}
}

func TestSaveGitSource_UpsertsAndPreservesCreatedAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}
	if err := db.SaveGitSource(ctx, first); err != nil {
		t.Fatalf("SaveGitSource(first) error = %v", err)
	}
	created, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}

	second := GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "release", BuildType: "railpack"}
	if err := db.SaveGitSource(ctx, second); err != nil {
		t.Fatalf("SaveGitSource(second) error = %v", err)
	}

	got, err := db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if got.Branch != "release" || got.BuildType != "railpack" {
		t.Errorf("GetGitSource() after upsert = %+v, want branch=release build_type=railpack", got)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want unchanged across upsert (%v)", got.CreatedAt, created.CreatedAt)
	}
}

func TestDeleteGitSource(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}
	if err := db.SaveGitSource(ctx, want); err != nil {
		t.Fatalf("SaveGitSource() error = %v", err)
	}
	if err := db.DeleteGitSource(ctx, "web"); err != nil {
		t.Fatalf("DeleteGitSource() error = %v", err)
	}

	_, err := db.GetGitSource(ctx, "web")
	if !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("GetGitSource() after delete error = %v, want ErrGitSourceNotFound", err)
	}
}

func TestDeleteGitSource_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteGitSource(ctx, "ghost")
	if !errors.Is(err, ErrGitSourceNotFound) {
		t.Fatalf("DeleteGitSource() error = %v, want ErrGitSourceNotFound", err)
	}
}

func TestGitSourceSecretsKey_DistinctFromServiceName(t *testing.T) {
	got := GitSourceSecretsKey("web")
	if got == "web" {
		t.Errorf("GitSourceSecretsKey(%q) = %q, must not equal the service name itself: a git source's credentials must not share a DEK with the app's own runtime env secrets", "web", got)
	}
}
