package apiclient

import (
	"context"
	"net/http"
)

// GitSSHKeyResource mirrors the API's public view of an app's deploy key.
// The private half never leaves the control plane.
type GitSSHKeyResource struct {
	PublicKey       string `json:"public_key"`
	Fingerprint     string `json:"fingerprint"`
	HostFingerprint string `json:"host_fingerprint,omitempty"`
	HostKeyPinned   bool   `json:"host_key_pinned"`
}

// SetGitSSHKeyRequest mirrors the API's setGitSSHKeyRequest.
type SetGitSSHKeyRequest struct {
	HostKey         string `json:"host_key,omitempty"`
	TrustOnFirstUse bool   `json:"trust_on_first_use,omitempty"`
}

func gitSSHKeyPath(name string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/git-source/ssh-key"
}

// GetGitSSHKey calls GET /api/v1/apps/{name}/git-source/ssh-key.
func (c *Client) GetGitSSHKey(ctx context.Context, name string) (GitSSHKeyResource, error) {
	var out GitSSHKeyResource
	err := c.do(ctx, http.MethodGet, gitSSHKeyPath(name), nil, &out)
	return out, err
}

// SetGitSSHKey calls POST: mints a fresh key (rotating any old one) and pins the host key.
func (c *Client) SetGitSSHKey(ctx context.Context, name string, req SetGitSSHKeyRequest) (GitSSHKeyResource, error) {
	var out GitSSHKeyResource
	err := c.do(ctx, http.MethodPost, gitSSHKeyPath(name), req, &out)
	return out, err
}

// DeleteGitSSHKey calls DELETE on the app's deploy key and pinned host key.
func (c *Client) DeleteGitSSHKey(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, gitSSHKeyPath(name), nil, nil)
}
