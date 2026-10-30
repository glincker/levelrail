package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// StackTrustRequest is the body of POST /api/v1/stacks/{name}/oidc-trusts.
type StackTrustRequest struct {
	Repository     string   `json:"repository"`
	RefPattern     string   `json:"ref_pattern"`
	Environment    string   `json:"environment,omitempty"`
	SubjectPattern string   `json:"subject_pattern,omitempty"`
	Verbs          []string `json:"verbs,omitempty"`
}

// StackTrust is one OIDC trust of a stack.
type StackTrust struct {
	ID             string   `json:"id"`
	Stack          string   `json:"stack"`
	Repository     string   `json:"repository"`
	RefPattern     string   `json:"ref_pattern"`
	Environment    string   `json:"environment"`
	SubjectPattern string   `json:"subject_pattern"`
	Verbs          []string `json:"verbs"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
}

// AddStackTrust calls POST /api/v1/stacks/{name}/oidc-trusts.
func (c *Client) AddStackTrust(ctx context.Context, name string, req StackTrustRequest) (StackTrust, json.RawMessage, error) {
	return stackCall[StackTrust](ctx, c, http.MethodPost, stackPath(name, "oidc-trusts"), req)
}

// ListStackTrusts calls GET /api/v1/stacks/{name}/oidc-trusts.
func (c *Client) ListStackTrusts(ctx context.Context, name string) ([]StackTrust, json.RawMessage, error) {
	out, raw, err := stackCall[struct {
		Trusts []StackTrust `json:"trusts"`
	}](ctx, c, http.MethodGet, stackPath(name, "oidc-trusts"), nil)
	return out.Trusts, raw, err
}

// RemoveStackTrust calls DELETE /api/v1/stacks/{name}/oidc-trusts/{id}.
func (c *Client) RemoveStackTrust(ctx context.Context, name, id string) error {
	_, _, err := stackCall[struct{}](ctx, c, http.MethodDelete, stackPath(name, "oidc-trusts", url.PathEscape(id)), nil)
	return err
}
