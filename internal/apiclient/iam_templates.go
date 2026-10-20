package apiclient

import (
	"context"
	"net/http"
)

// PolicyTemplateParam mirrors internal/api's policyTemplateParam.
type PolicyTemplateParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// PolicyStatement is one Allow or Deny rule of a policy document.
type PolicyStatement struct {
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

// PolicyDocument is a policy body, typed so tool output schemas stay exact.
type PolicyDocument struct {
	Statement []PolicyStatement `json:"Statement"`
}

// PolicyTemplate mirrors internal/api's policyTemplateResource.
type PolicyTemplate struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Params      []PolicyTemplateParam `json:"params"`
	Document    PolicyDocument        `json:"document"`
}

// PolicyTemplateList mirrors internal/api's policyTemplateListResponse.
type PolicyTemplateList struct {
	Version   int              `json:"version"`
	Templates []PolicyTemplate `json:"templates"`
}

// ApplyPolicyTemplateRequest mirrors internal/api's applyTemplateRequest.
type ApplyPolicyTemplateRequest struct {
	Name   string                     `json:"name,omitempty"`
	Params map[string]string          `json:"params,omitempty"`
	Attach *ApplyPolicyTemplateAttach `json:"attach,omitempty"`
}

// ApplyPolicyTemplateAttach names the user or token a template policy is attached to.
type ApplyPolicyTemplateAttach struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

// ApplyPolicyTemplateResponse mirrors internal/api's applyTemplateResponse.
type ApplyPolicyTemplateResponse struct {
	Policy   PolicyResource `json:"policy"`
	Attached bool           `json:"attached"`
}

const policyTemplatesPath = "/api/v1/iam/policy-templates"

// ListPolicyTemplates calls GET /api/v1/iam/policy-templates.
func (c *Client) ListPolicyTemplates(ctx context.Context) (PolicyTemplateList, error) {
	var out PolicyTemplateList
	err := c.do(ctx, http.MethodGet, policyTemplatesPath, nil, &out)
	return out, err
}

// ApplyPolicyTemplate calls POST /api/v1/iam/policy-templates/{id}/apply.
func (c *Client) ApplyPolicyTemplate(ctx context.Context, id string, req ApplyPolicyTemplateRequest) (ApplyPolicyTemplateResponse, error) {
	var out ApplyPolicyTemplateResponse
	err := c.do(ctx, http.MethodPost, policyTemplatesPath+"/"+PathEscape(id)+"/apply", req, &out)
	return out, err
}
