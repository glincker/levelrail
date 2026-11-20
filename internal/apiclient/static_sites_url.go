package apiclient

import (
	"context"
	"net/http"
)

// CreateStaticSiteFromURL calls POST /api/v1/static-sites with an https archive URL as the first deploy.
func (c *Client) CreateStaticSiteFromURL(ctx context.Context, name, sourceURL string) (StaticSiteCreated, error) {
	var out StaticSiteCreated
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites", map[string]any{"name": name, "source_url": sourceURL}, &out)
	return out, err
}

// DeployStaticSiteFromURL redeploys an existing site from an https archive URL.
func (c *Client) DeployStaticSiteFromURL(ctx context.Context, name, sourceURL string) (BuildTriggerResponse, error) {
	var out BuildTriggerResponse
	body := map[string]any{"source_url": sourceURL, "build": map[string]string{"type": "static"}}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/builds", body, &out)
	return out, err
}
