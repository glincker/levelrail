package apiclient

import (
	"context"
	"net/http"
	"time"
)

// CanaryResource mirrors internal/api's canaryResource.
type CanaryResource struct {
	Active    bool       `json:"active"`
	Image     string     `json:"image,omitempty"`
	Weight    int        `json:"weight"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

func canaryPath(appName string) string { return "/api/v1/apps/" + PathEscape(appName) + "/canary" }

// GetCanary calls GET /api/v1/apps/{name}/canary.
func (c *Client) GetCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodGet, canaryPath(appName), nil, &out)
	return out, err
}

// StartCanary calls POST /api/v1/apps/{name}/canary. A weight of 0 uses the server default.
func (c *Client) StartCanary(ctx context.Context, appName, image string, weight int) (CanaryResource, error) {
	body := map[string]any{"image": image}
	if weight > 0 {
		body["weight"] = weight
	}
	var out CanaryResource
	err := c.do(ctx, http.MethodPost, canaryPath(appName), body, &out)
	return out, err
}

// SetCanaryWeight calls PUT /api/v1/apps/{name}/canary; 0 pauses the canary.
func (c *Client) SetCanaryWeight(ctx context.Context, appName string, weight int) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodPut, canaryPath(appName), map[string]int{"weight": weight}, &out)
	return out, err
}

// PromoteCanary calls POST /api/v1/apps/{name}/canary/promote.
func (c *Client) PromoteCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodPost, canaryPath(appName)+"/promote", nil, &out)
	return out, err
}

// AbortCanary calls DELETE /api/v1/apps/{name}/canary.
func (c *Client) AbortCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodDelete, canaryPath(appName), nil, &out)
	return out, err
}
