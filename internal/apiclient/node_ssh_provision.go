package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SSHNodeProvisionAuthRequest mirrors internal/api's
// createSSHNodeProvisionAuthRequest: one of two credential shapes, never
// stored past the one provisioning call this starts.
type SSHNodeProvisionAuthRequest struct {
	Type       string `json:"type"` // "key" or "password"
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Password   string `json:"password,omitempty"`
}

// CreateSSHNodeProvisionRequest is POST /api/v1/nodes/ssh-provision's body.
type CreateSSHNodeProvisionRequest struct {
	Host             string                      `json:"host"`
	Port             int                         `json:"port,omitempty"`
	Username         string                      `json:"username"`
	Auth             SSHNodeProvisionAuthRequest `json:"auth"`
	Name             string                      `json:"name"`
	Role             string                      `json:"role,omitempty"`
	ControlPlaneAddr string                      `json:"control_plane_addr"`
}

// SSHNodeProvisionResource mirrors internal/api's sshNodeProvisionResource.
type SSHNodeProvisionResource struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	DetectedOS    string    `json:"detected_os,omitempty"`
	DetectedArch  string    `json:"detected_arch,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	Log           string    `json:"log,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateSSHNodeProvision calls POST /api/v1/nodes/ssh-provision.
func (c *Client) CreateSSHNodeProvision(ctx context.Context, req CreateSSHNodeProvisionRequest) (SSHNodeProvisionResource, error) {
	var out SSHNodeProvisionResource
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/ssh-provision", req, &out)
	return out, err
}

func sshNodeProvisionsPath() string { return "/api/v1/ssh-node-provisions" }

// ListSSHNodeProvisions calls GET /api/v1/ssh-node-provisions.
func (c *Client) ListSSHNodeProvisions(ctx context.Context) ([]SSHNodeProvisionResource, error) {
	var out []SSHNodeProvisionResource
	err := c.do(ctx, http.MethodGet, sshNodeProvisionsPath(), nil, &out)
	return out, err
}

// GetSSHNodeProvision calls GET /api/v1/ssh-node-provisions/{id}.
func (c *Client) GetSSHNodeProvision(ctx context.Context, id string) (SSHNodeProvisionResource, error) {
	var out SSHNodeProvisionResource
	err := c.do(ctx, http.MethodGet, sshNodeProvisionsPath()+"/"+PathEscape(id), nil, &out)
	return out, err
}
