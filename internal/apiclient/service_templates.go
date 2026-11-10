package apiclient

import (
	"context"
	"net/http"
)

// ValidateTemplateRequest is POST /api/v1/service-templates/validate's body.
type ValidateTemplateRequest struct {
	TemplateYAML string `json:"template_yaml"`
	ComposeYAML  string `json:"compose_yaml"`
}

// ValidateTemplateResult mirrors the server's validateTemplateResponse.
type ValidateTemplateResult struct {
	Valid  bool     `json:"valid"`
	ID     string   `json:"id,omitempty"`
	Name   string   `json:"name,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

// ValidateServiceTemplate calls POST /api/v1/service-templates/validate.
func (c *Client) ValidateServiceTemplate(ctx context.Context, req ValidateTemplateRequest) (ValidateTemplateResult, error) {
	var out ValidateTemplateResult
	err := c.do(ctx, http.MethodPost, "/api/v1/service-templates/validate", req, &out)
	return out, err
}

// DeployServiceTemplateNow calls POST /api/v1/service-templates/{id}/deploy:
// the one-click trial deploy under a server generated app name.
func (c *Client) DeployServiceTemplateNow(ctx context.Context, id string) (ComposeStackDeploy, error) {
	var out ComposeStackDeploy
	err := c.do(ctx, http.MethodPost, "/api/v1/service-templates/"+PathEscape(id)+"/deploy", nil, &out)
	return out, err
}
