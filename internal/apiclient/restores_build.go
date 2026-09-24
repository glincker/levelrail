package apiclient

import (
	"context"
	"net/http"
)

// DetectFrameworkResource mirrors internal/api's detectFrameworkResponse.
type DetectFrameworkResource struct {
	Provider      string `json:"provider,omitempty"`
	FrameworkName string `json:"framework_name,omitempty"`
	Detected      bool   `json:"detected"`
}

// RepoBranchesResource mirrors internal/api's gitBranchesResponse.
type RepoBranchesResource struct {
	Branches []string `json:"branches"`
}

type repoRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref,omitempty"`
}

// DetectFramework calls POST /api/v1/build/detect.
func (c *Client) DetectFramework(ctx context.Context, repoURL, ref string) (DetectFrameworkResource, error) {
	var out DetectFrameworkResource
	err := c.do(ctx, http.MethodPost, "/api/v1/build/detect", repoRequest{RepoURL: repoURL, Ref: ref}, &out)
	return out, err
}

// ListRepoBranches calls the repo branch listing route (POST under /api/v1/git).
func (c *Client) ListRepoBranches(ctx context.Context, repoURL string) (RepoBranchesResource, error) {
	var out RepoBranchesResource
	err := c.do(ctx, http.MethodPost, "/api/v1/git/branches", repoRequest{RepoURL: repoURL}, &out)
	return out, err
}

// ListRestores calls GET /api/v1/databases/{name}/restores.
func (c *Client) ListRestores(ctx context.Context, name string) ([]RestoreHistoryResource, error) {
	var out []RestoreHistoryResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/restores", nil, &out)
	return out, err
}

// ListVolumeRestores calls GET /api/v1/apps/{name}/volumes/{volume}/restores.
func (c *Client) ListVolumeRestores(ctx context.Context, name, volume string) ([]RestoreHistoryResource, error) {
	var out []RestoreHistoryResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/volumes/"+PathEscape(volume)+"/restores", nil, &out)
	return out, err
}

// ListPITRRestores calls GET /api/v1/databases/{name}/pitr-restores.
func (c *Client) ListPITRRestores(ctx context.Context, name string) ([]PITRRestoreHistoryResource, error) {
	var out []PITRRestoreHistoryResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/pitr-restores", nil, &out)
	return out, err
}
