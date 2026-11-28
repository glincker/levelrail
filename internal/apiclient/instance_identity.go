package apiclient

import (
	"context"
	"net/http"
)

// InstanceIdentity mirrors the signed-in GET /api/v1/instance/identity view.
type InstanceIdentity struct {
	InstanceInfo
	Moved              bool   `json:"moved"`
	MovedAt            string `json:"moved_at,omitempty"`
	PreviousInstanceID string `json:"previous_instance_id,omitempty"`
}

// GetInstanceIdentity calls GET /api/v1/instance/identity.
func (c *Client) GetInstanceIdentity(ctx context.Context) (InstanceIdentity, error) {
	var out InstanceIdentity
	err := c.do(ctx, http.MethodGet, "/api/v1/instance/identity", nil, &out)
	return out, err
}
