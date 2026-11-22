package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
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
	return getJSON[AuthEngineStatusResource](ctx, c, "/api/v1/auth-engine/status")
}

// RotateToken calls POST /api/v1/auth/tokens/{id}/rotate.
func (c *Client) RotateToken(ctx context.Context, id string) (RotatedToken, error) {
	var out RotatedToken
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/tokens/"+url.PathEscape(id)+"/rotate", nil, &out)
	return out, err
}

// ListTokensRaw calls GET /api/v1/auth/tokens with the client's bearer token (root ability), returning the JSON array undecoded.
func (c *Client) ListTokensRaw(ctx context.Context) (json.RawMessage, error) {
	return getJSON[json.RawMessage](ctx, c, "/api/v1/auth/tokens")
}
