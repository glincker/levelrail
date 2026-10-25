package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// PullRequest is the slice of a pull request that comment commands need.
type PullRequest struct {
	Number           int
	State            string
	Merged           bool
	BaseRef          string
	HeadRef          string
	HeadSHA          string
	HeadRepoFullName string
	BaseRepoFullName string
}

type pullRequestResponse struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Head   struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

// GetPullRequest reads pull request number of owner/repo. A deleted head
// repository comes back with an empty HeadRepoFullName.
func (c *Client) GetPullRequest(ctx context.Context, instanceURL, token, owner, repo string, number int) (PullRequest, error) {
	var resp pullRequestResponse
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	if err := c.do(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+token, &resp); err != nil {
		return PullRequest{}, err
	}
	pr := PullRequest{Number: resp.Number, State: resp.State, Merged: resp.Merged, BaseRef: resp.Base.Ref, HeadRef: resp.Head.Ref, HeadSHA: resp.Head.SHA}
	if resp.Head.Repo != nil {
		pr.HeadRepoFullName = resp.Head.Repo.FullName
	}
	if resp.Base.Repo != nil {
		pr.BaseRepoFullName = resp.Base.Repo.FullName
	}
	return pr, nil
}

type collaboratorPermissionResponse struct {
	Permission string `json:"permission"`
}

// GetCollaboratorPermission returns login's repository permission: one of
// admin, write, read or none (GitHub folds maintain into write and triage into read).
func (c *Client) GetCollaboratorPermission(ctx context.Context, instanceURL, token, owner, repo, login string) (string, error) {
	var resp collaboratorPermissionResponse
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(login))
	if err := c.do(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+token, &resp); err != nil {
		return "", err
	}
	return resp.Permission, nil
}
