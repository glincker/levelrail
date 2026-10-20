package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DeployPolicyRule mirrors internal/policy.Rule.
type DeployPolicyRule struct {
	Mode   string   `json:"mode"`
	Values []string `json:"values,omitempty"`
	Limit  float64  `json:"limit,omitempty"`
}

// DeployPolicy mirrors internal/api's deployPolicyResource.
type DeployPolicy struct {
	Scope     string                      `json:"scope"`
	ScopeID   string                      `json:"scope_id"`
	Rules     map[string]DeployPolicyRule `json:"rules"`
	UpdatedBy string                      `json:"updated_by,omitempty"`
	UpdatedAt *time.Time                  `json:"updated_at,omitempty"`
}

// PolicyViolation mirrors internal/policy.Violation.
type PolicyViolation struct {
	Rule       string `json:"rule"`
	Code       string `json:"code"`
	Mode       string `json:"mode"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}

// PolicyCheckApp is one app's dry-run outcome.
type PolicyCheckApp struct {
	App         string            `json:"app"`
	ProjectID   string            `json:"project_id,omitempty"`
	Environment string            `json:"environment_id,omitempty"`
	Violations  []PolicyViolation `json:"violations"`
	WouldFail   bool              `json:"would_fail"`
}

// PolicyCheckResult is the body of POST /api/v1/deploy-policies/check.
type PolicyCheckResult struct {
	Checked int              `json:"checked"`
	Failing int              `json:"failing"`
	Warning int              `json:"warning"`
	Apps    []PolicyCheckApp `json:"apps"`
}

// PolicyCheckRequest narrows a dry run to one app and previews unsaved rules.
type PolicyCheckRequest struct {
	App     string                      `json:"app,omitempty"`
	Scope   string                      `json:"scope,omitempty"`
	ScopeID string                      `json:"scope_id,omitempty"`
	Rules   map[string]DeployPolicyRule `json:"rules,omitempty"`
}

// ListDeployPolicies calls GET /api/v1/deploy-policies.
func (c *Client) ListDeployPolicies(ctx context.Context) ([]DeployPolicy, error) {
	var out []DeployPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/deploy-policies", nil, &out)
	return out, err
}

// SetDeployPolicy calls PUT /api/v1/deploy-policies/{scope}/{id}.
func (c *Client) SetDeployPolicy(ctx context.Context, scope, id string, rules map[string]DeployPolicyRule) (DeployPolicy, error) {
	var out DeployPolicy
	body := struct {
		Rules map[string]DeployPolicyRule `json:"rules"`
	}{rules}
	err := c.do(ctx, http.MethodPut, "/api/v1/deploy-policies/"+PathEscape(scope)+"/"+PathEscape(id), body, &out)
	return out, err
}

// DeleteDeployPolicy calls DELETE /api/v1/deploy-policies/{scope}/{id}.
func (c *Client) DeleteDeployPolicy(ctx context.Context, scope, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/deploy-policies/"+PathEscape(scope)+"/"+PathEscape(id), nil, nil)
}

// CheckDeployPolicy calls POST /api/v1/deploy-policies/check.
func (c *Client) CheckDeployPolicy(ctx context.Context, req PolicyCheckRequest) (PolicyCheckResult, error) {
	var out PolicyCheckResult
	err := c.do(ctx, http.MethodPost, "/api/v1/deploy-policies/check", req, &out)
	return out, err
}
