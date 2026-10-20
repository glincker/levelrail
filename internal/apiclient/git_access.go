package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// GitAccessCheck is one step of a git access validation.
type GitAccessCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// GitAccessFix is the typed next step for a failed validation.
type GitAccessFix struct {
	Kind string `json:"kind"`
	Code string `json:"code"`
	Hint string `json:"hint"`
	Link string `json:"link,omitempty"`
}

// GitAccessResult is the result of validating repo or provider access.
type GitAccessResult struct {
	OK         bool             `json:"ok"`
	Credential string           `json:"credential,omitempty"`
	Branch     string           `json:"branch,omitempty"`
	HeadSHA    string           `json:"head_sha,omitempty"`
	Checks     []GitAccessCheck `json:"checks"`
	Fix        *GitAccessFix    `json:"fix,omitempty"`
}

// ValidateGitSourceAccess calls POST /api/v1/apps/{name}/git-source/validate.
func (c *Client) ValidateGitSourceAccess(ctx context.Context, app string) (GitAccessResult, error) {
	var out GitAccessResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+url.PathEscape(app)+"/git-source/validate", nil, &out)
	return out, err
}

// ValidateGitProvider calls POST /api/v1/git-providers/{provider}/validate.
func (c *Client) ValidateGitProvider(ctx context.Context, provider string) (GitAccessResult, error) {
	var out GitAccessResult
	err := c.do(ctx, http.MethodPost, "/api/v1/git-providers/"+url.PathEscape(provider)+"/validate", nil, &out)
	return out, err
}
