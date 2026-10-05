package apiclient

import (
	"context"
	"net/http"
)

// AuthEngineStatusResource mirrors internal/api's authEngineStatusResponse.
type AuthEngineStatusResource struct {
	LibraryVersion string `json:"library_version"`
	TOTP           bool   `json:"totp"`
	Passkeys       bool   `json:"passkeys"`
	OAuth          bool   `json:"oauth"`
}

// GetAuthEngineStatus calls GET /api/v1/auth-engine/status (root only).
func (c *Client) GetAuthEngineStatus(ctx context.Context) (AuthEngineStatusResource, error) {
	var out AuthEngineStatusResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth-engine/status", nil, &out)
	return out, err
}
