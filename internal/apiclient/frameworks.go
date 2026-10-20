package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/frameworks"
)

type frameworkListResource struct {
	Frameworks []frameworks.Preset `json:"frameworks"`
}

type frameworkDockerfileResource struct {
	Dockerfile string `json:"dockerfile"`
}

// ListFrameworks calls GET /api/v1/frameworks.
func (c *Client) ListFrameworks(ctx context.Context) ([]frameworks.Preset, error) {
	var out frameworkListResource
	err := c.do(ctx, http.MethodGet, "/api/v1/frameworks", nil, &out)
	return out.Frameworks, err
}

// GetFramework calls GET /api/v1/frameworks/{id}.
func (c *Client) GetFramework(ctx context.Context, id string) (frameworks.Preset, error) {
	var out frameworks.Preset
	err := c.do(ctx, http.MethodGet, "/api/v1/frameworks/"+url.PathEscape(id), nil, &out)
	return out, err
}

// GetFrameworkDockerfile calls GET /api/v1/frameworks/{id}/dockerfile.
func (c *Client) GetFrameworkDockerfile(ctx context.Context, id string) (string, error) {
	var out frameworkDockerfileResource
	err := c.do(ctx, http.MethodGet, "/api/v1/frameworks/"+url.PathEscape(id)+"/dockerfile", nil, &out)
	return out.Dockerfile, err
}
