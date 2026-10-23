package repolayout

import (
	"context"
	"net/url"
)

type defaultBranchResponse struct {
	DefaultBranch string `json:"default_branch"`
}

// DefaultBranch asks the forge for the repository's default branch.
func (s *githubSource) DefaultBranch(ctx context.Context) (string, error) {
	var out defaultBranchResponse
	if err := getJSON(ctx, s.c, s.api+"/repos/"+escapePath(s.repo), s.headers("application/vnd.github+json"), &out); err != nil {
		return "", err
	}
	return out.DefaultBranch, nil
}

// DefaultBranch asks the forge for the project's default branch.
func (s *gitlabSource) DefaultBranch(ctx context.Context) (string, error) {
	var out defaultBranchResponse
	if err := getJSON(ctx, s.c, s.api+"/projects/"+url.PathEscape(s.repo), s.headers(), &out); err != nil {
		return "", err
	}
	return out.DefaultBranch, nil
}

// DefaultBranch asks Gitea or Forgejo for the repository's default branch.
func (s *giteaSource) DefaultBranch(ctx context.Context) (string, error) {
	var out defaultBranchResponse
	if err := getJSON(ctx, s.c, s.api+"/repos/"+escapePath(s.repo), s.headers(), &out); err != nil {
		return "", err
	}
	return out.DefaultBranch, nil
}
