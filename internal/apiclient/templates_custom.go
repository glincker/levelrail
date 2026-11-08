package apiclient

import (
	"context"
	"net/http"
)

// ImportCustomTemplateRequest mirrors internal/api's importCustomTemplateRequest.
type ImportCustomTemplateRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ComposeYAML string   `json:"compose_yaml"`
	Variables   []string `json:"variables,omitempty"`
}

// ImportCustomTemplateResult mirrors internal/api's importCustomTemplateResponse.
type ImportCustomTemplateResult struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	RequiresConfiguration bool     `json:"requires_configuration"`
	RequiredEnvKeys       []string `json:"required_env_keys,omitempty"`
	CreatedAt             string   `json:"created_at"`
}

// ImportCustomTemplate calls POST /api/v1/templates/custom.
func (c *Client) ImportCustomTemplate(ctx context.Context, req ImportCustomTemplateRequest) (ImportCustomTemplateResult, error) {
	var out ImportCustomTemplateResult
	err := c.do(ctx, http.MethodPost, "/api/v1/templates/custom", req, &out)
	return out, err
}
