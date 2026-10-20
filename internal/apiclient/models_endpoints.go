package apiclient

import (
	"context"
	"net/http"
	"time"
)

// ExternalEndpointRequest mirrors internal/api's externalRequest.
type ExternalEndpointRequest struct {
	BaseURL          string `json:"base_url"`
	APIType          string `json:"api_type,omitempty"`
	Credential       string `json:"credential,omitempty"`
	FallbackModel    string `json:"fallback_model,omitempty"`
	FallbackModelRef string `json:"fallback_model_ref,omitempty"`
	LatencyBudgetMS  int64  `json:"latency_budget_ms,omitempty"`
}

// EndpointDiagnosis mirrors internal/api's endpointDiagnosis.
type EndpointDiagnosis struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// EndpointHealth mirrors internal/api's endpointHealthResource.
type EndpointHealth struct {
	State     string             `json:"state"`
	LatencyMS int64              `json:"latency_ms"`
	CheckedAt *time.Time         `json:"checked_at,omitempty"`
	LastOKAt  *time.Time         `json:"last_ok_at,omitempty"`
	Failures  int                `json:"consecutive_failures"`
	Diagnosis *EndpointDiagnosis `json:"diagnosis,omitempty"`
}

// ExternalEndpoint mirrors internal/api's endpointResource.
type ExternalEndpoint struct {
	BaseURL          string         `json:"base_url"`
	APIType          string         `json:"api_type"`
	CredentialSet    bool           `json:"credential_set"`
	FallbackModel    string         `json:"fallback_model,omitempty"`
	FallbackModelRef string         `json:"fallback_model_ref,omitempty"`
	LatencyBudgetMS  int64          `json:"latency_budget_ms"`
	FallbackCount    int            `json:"fallback_count"`
	Models           []string       `json:"models"`
	DiscoveryError   string         `json:"discovery_error,omitempty"`
	PrivateAllowed   bool           `json:"private_allowed"`
	Health           EndpointHealth `json:"health"`
}

// EndpointTest mirrors internal/api's endpointTestResponse.
type EndpointTest struct {
	State     string             `json:"state"`
	LatencyMS int64              `json:"latency_ms"`
	Models    []string           `json:"models"`
	Diagnosis *EndpointDiagnosis `json:"diagnosis,omitempty"`
}

// TestEndpointRequest probes an endpoint before it is registered.
type TestEndpointRequest struct {
	BaseURL    string `json:"base_url"`
	APIType    string `json:"api_type,omitempty"`
	Credential string `json:"credential,omitempty"`
}

// TestModelEndpointInput calls POST /api/v1/model-endpoints/test.
func (c *Client) TestModelEndpointInput(ctx context.Context, req TestEndpointRequest) (EndpointTest, error) {
	var out EndpointTest
	err := c.do(ctx, http.MethodPost, "/api/v1/model-endpoints/test", req, &out)
	return out, err
}

// TestModelEndpoint calls POST /api/v1/models/{name}/endpoint/test.
func (c *Client) TestModelEndpoint(ctx context.Context, name string) (EndpointTest, error) {
	var out EndpointTest
	err := c.do(ctx, http.MethodPost, "/api/v1/models/"+PathEscape(name)+"/endpoint/test", nil, &out)
	return out, err
}

// SetModelEndpointRouting calls PUT /api/v1/models/{name}/endpoint/routing.
// An empty fallback turns the policy off.
func (c *Client) SetModelEndpointRouting(ctx context.Context, name, fallback, fallbackRef string, budgetMS int64) (ModelDetail, error) {
	body := map[string]any{"fallback_model": fallback, "fallback_model_ref": fallbackRef, "latency_budget_ms": budgetMS}
	var out ModelDetail
	err := c.do(ctx, http.MethodPut, "/api/v1/models/"+PathEscape(name)+"/endpoint/routing", body, &out)
	return out, err
}

// SetModelEndpointCredential calls PUT /api/v1/models/{name}/endpoint/credential.
// An empty credential removes it.
func (c *Client) SetModelEndpointCredential(ctx context.Context, name, credential string) (ModelDetail, error) {
	var out ModelDetail
	err := c.do(ctx, http.MethodPut, "/api/v1/models/"+PathEscape(name)+"/endpoint/credential", map[string]string{"credential": credential}, &out)
	return out, err
}
