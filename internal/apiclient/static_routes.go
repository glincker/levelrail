package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

// StaticRoutesResource is a static site's effective routing rules.
type StaticRoutesResource struct {
	staticroutes.Ruleset
	Published bool `json:"published"`
}

// StaticRoutesTestRequest is one request to run through a site's rules.
type StaticRoutesTestRequest struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
}

// StaticRoutesLintRequest is pasted _redirects and _headers text.
type StaticRoutesLintRequest struct {
	Redirects string `json:"redirects"`
	Headers   string `json:"headers"`
}

// StaticRoutesLintResource is the parse result of a lint request.
type StaticRoutesLintResource struct {
	staticroutes.Ruleset
	Error string `json:"error,omitempty"`
}

// GetStaticRoutes calls GET /api/v1/apps/{name}/static-routes.
func (c *Client) GetStaticRoutes(ctx context.Context, name string) (StaticRoutesResource, error) {
	var out StaticRoutesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/static-routes", nil, &out)
	return out, err
}

// TestStaticRoutes calls POST /api/v1/apps/{name}/static-routes/test.
func (c *Client) TestStaticRoutes(ctx context.Context, name string, req StaticRoutesTestRequest) (staticroutes.Decision, error) {
	var out staticroutes.Decision
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/static-routes/test", req, &out)
	return out, err
}

// LintStaticRoutes calls POST /api/v1/apps/{name}/static-routes/lint.
func (c *Client) LintStaticRoutes(ctx context.Context, name string, req StaticRoutesLintRequest) (StaticRoutesLintResource, error) {
	var out StaticRoutesLintResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/static-routes/lint", req, &out)
	return out, err
}
