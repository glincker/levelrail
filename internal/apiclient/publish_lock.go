package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PublishLockResource is an app's publish lock state.
type PublishLockResource struct {
	Locked      bool      `json:"locked"`
	App         string    `json:"app,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	LockedBy    string    `json:"locked_by,omitempty"`
	LockedAt    time.Time `json:"locked_at,omitempty"`
	PinnedImage string    `json:"pinned_image,omitempty"`
	HeldBuilds  []string  `json:"held_builds"`
}

type putPublishLockRequest struct {
	Reason string `json:"reason"`
}

func publishLockPath(appName string) string {
	return "/api/v1/apps/" + PathEscape(appName) + "/publish-lock"
}

// GetPublishLock calls GET /api/v1/apps/{name}/publish-lock.
func (c *Client) GetPublishLock(ctx context.Context, appName string) (PublishLockResource, error) {
	var out PublishLockResource
	err := c.do(ctx, http.MethodGet, publishLockPath(appName), nil, &out)
	return out, err
}

// LockPublishing calls PUT /api/v1/apps/{name}/publish-lock.
func (c *Client) LockPublishing(ctx context.Context, appName, reason string) (PublishLockResource, error) {
	var out PublishLockResource
	err := c.do(ctx, http.MethodPut, publishLockPath(appName), putPublishLockRequest{Reason: reason}, &out)
	return out, err
}

// UnlockPublishing calls DELETE /api/v1/apps/{name}/publish-lock.
func (c *Client) UnlockPublishing(ctx context.Context, appName string) (PublishLockResource, error) {
	var out PublishLockResource
	err := c.do(ctx, http.MethodDelete, publishLockPath(appName), nil, &out)
	return out, err
}

// PublishHeldBuild calls POST /api/v1/apps/{name}/deploys/{id}/publish.
func (c *Client) PublishHeldBuild(ctx context.Context, appName, deployID string) (DeployTriggerResult, error) {
	var out DeployTriggerResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(appName)+"/deploys/"+PathEscape(deployID)+"/publish", nil, &out)
	return out, err
}

// RollbackAndLockDeploy calls POST /api/v1/apps/{name}/deploys/{id}/rollback-lock.
func (c *Client) RollbackAndLockDeploy(ctx context.Context, appName, deployID string, req RollbackToRequest) (DeployTriggerResult, error) {
	var out DeployTriggerResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(appName)+"/deploys/"+PathEscape(deployID)+"/rollback-lock", req, &out)
	return out, err
}
