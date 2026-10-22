package apiclient

import (
	"context"
	"net/http"
)

// ProvisionStepResource is one step of a database creation attempt.
type ProvisionStepResource struct {
	Step    string `json:"step"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Current int64  `json:"current,omitempty"`
	Total   int64  `json:"total,omitempty"`
}

// ProvisionFixResource is a typed recovery action for a failed step.
type ProvisionFixResource struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Confirm bool   `json:"confirm,omitempty"`
}

// ProvisionFailureResource says why an attempt stopped.
type ProvisionFailureResource struct {
	Step   string                `json:"step"`
	Reason string                `json:"reason"`
	Fix    *ProvisionFixResource `json:"fix,omitempty"`
}

// ProvisionAttemptResource is the latest creation attempt of a database.
type ProvisionAttemptResource struct {
	ID          string                    `json:"attempt_id"`
	Status      string                    `json:"status"`
	CurrentStep string                    `json:"current_step,omitempty"`
	Steps       []ProvisionStepResource   `json:"steps"`
	Failure     *ProvisionFailureResource `json:"failure,omitempty"`
}

// GetDatabaseProvision calls GET /api/v1/databases/{name}/provision.
func (c *Client) GetDatabaseProvision(ctx context.Context, name string) (ProvisionAttemptResource, error) {
	var out ProvisionAttemptResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/provision", nil, &out)
	return out, err
}
