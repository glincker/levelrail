package apiclient

import (
	"context"
	"net/http"
)

// DeployFromCacheRequest is the body of POST /api/v1/apps/{name}/deploys/from-cache.
// Commit defaults to the app's last successful source deploy.
type DeployFromCacheRequest struct {
	Commit         string `json:"commit,omitempty"`
	OverrideFreeze bool   `json:"override_freeze,omitempty"`
	OverrideReason string `json:"override_reason,omitempty"`
}

// DeployFromCache calls POST /api/v1/apps/{name}/deploys/from-cache: a rebuild
// from the control plane's source cache that never contacts the git host.
func (c *Client) DeployFromCache(ctx context.Context, name string, req DeployFromCacheRequest) (BuildTriggerResponse, error) {
	var out BuildTriggerResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/deploys/from-cache", req, &out)
	return out, err
}
