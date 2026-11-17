package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// K8sRemoveResult is the response of DELETE /api/v1/clusters/{name}/deployments/{app}.
type K8sRemoveResult struct {
	App       string   `json:"app"`
	Namespace string   `json:"namespace"`
	Deleted   []string `json:"deleted"`
	Absent    []string `json:"absent"`
}

// GetK8sDeploymentLogs calls GET /api/v1/clusters/{name}/deployments/{app}/logs and returns the text.
func (c *Client) GetK8sDeploymentLogs(ctx context.Context, name, app, pod string, tail int) (string, error) {
	q := url.Values{}
	if pod != "" {
		q.Set("pod", pod)
	}
	if tail > 0 {
		q.Set("tail", strconv.Itoa(tail))
	}
	path := k8sClusterPath(name) + "/deployments/" + PathEscape(app) + "/logs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	b, err := c.rawRequest(ctx, http.MethodGet, path, nil)
	return string(b), err
}

// RemoveK8sDeployment calls DELETE /api/v1/clusters/{name}/deployments/{app}.
func (c *Client) RemoveK8sDeployment(ctx context.Context, name, app string) (K8sRemoveResult, error) {
	var out K8sRemoveResult
	err := c.do(ctx, http.MethodDelete, k8sClusterPath(name)+"/deployments/"+PathEscape(app), nil, &out)
	return out, err
}
