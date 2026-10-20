package apiclient

import (
	"context"
	"net/http"
	"time"
)

// GitRemote is an app's push deploy remote as the API reports it. Token and
// AddCommand are set only on the call that issued the token.
type GitRemote struct {
	Configured     bool       `json:"configured"`
	URL            string     `json:"url"`
	RemoteName     string     `json:"remote_name"`
	Username       string     `json:"username"`
	Branch         string     `json:"branch"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	RotatedAt      *time.Time `json:"rotated_at,omitempty"`
	LastPushAt     *time.Time `json:"last_push_at,omitempty"`
	LastPushCommit string     `json:"last_push_commit,omitempty"`
	LastPushResult string     `json:"last_push_result,omitempty"`
	MaxPushBytes   int64      `json:"max_push_bytes"`
	Token          string     `json:"token,omitempty"`
	AddCommand     string     `json:"add_command,omitempty"`
}

// GetGitRemote calls GET /api/v1/apps/{name}/git-remote.
func (c *Client) GetGitRemote(ctx context.Context, name string) (GitRemote, error) {
	var out GitRemote
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/git-remote", nil, &out)
	return out, err
}

// IssueGitRemote calls POST /api/v1/apps/{name}/git-remote: it enables the
// remote or rotates its token, and returns the new token once.
func (c *Client) IssueGitRemote(ctx context.Context, name string) (GitRemote, error) {
	var out GitRemote
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/git-remote", nil, &out)
	return out, err
}

// SetGitRemoteBranch calls PUT /api/v1/apps/{name}/git-remote.
func (c *Client) SetGitRemoteBranch(ctx context.Context, name, branch string) (GitRemote, error) {
	var out GitRemote
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/git-remote", map[string]string{"branch": branch}, &out)
	return out, err
}

// DeleteGitRemote calls DELETE /api/v1/apps/{name}/git-remote, which revokes the token.
func (c *Client) DeleteGitRemote(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/git-remote", nil, nil)
}
