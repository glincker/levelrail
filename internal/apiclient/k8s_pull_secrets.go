package apiclient

import (
	"context"
	"net/http"
)

// K8sPullSecret names a stored registry credential. The API never returns the credential.
type K8sPullSecret struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
}

// AddK8sPullSecretRequest is the body of POST /clusters/{name}/pull-secrets.
type AddK8sPullSecretRequest struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// AddK8sPullSecret calls POST /api/v1/clusters/{name}/pull-secrets.
func (c *Client) AddK8sPullSecret(ctx context.Context, cluster string, req AddK8sPullSecretRequest) (K8sPullSecret, error) {
	var out K8sPullSecret
	err := c.do(ctx, http.MethodPost, k8sClusterPath(cluster)+"/pull-secrets", req, &out)
	return out, err
}

// ListK8sPullSecrets calls GET /api/v1/clusters/{name}/pull-secrets.
func (c *Client) ListK8sPullSecrets(ctx context.Context, cluster string) ([]K8sPullSecret, error) {
	var out struct {
		PullSecrets []K8sPullSecret `json:"pull_secrets"`
	}
	err := c.do(ctx, http.MethodGet, k8sClusterPath(cluster)+"/pull-secrets", nil, &out)
	return out.PullSecrets, err
}

// RemoveK8sPullSecret calls DELETE /api/v1/clusters/{name}/pull-secrets/{secret}.
func (c *Client) RemoveK8sPullSecret(ctx context.Context, cluster, name string) error {
	return c.do(ctx, http.MethodDelete, k8sClusterPath(cluster)+"/pull-secrets/"+PathEscape(name), nil, nil)
}
