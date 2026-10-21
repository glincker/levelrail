package apiclient

import (
	"context"
	"net/http"
	"time"
)

// CLIPairRequest is the body of POST /api/v1/auth/cli-pairing/redeem.
type CLIPairRequest struct {
	Code       string `json:"code"`
	ClientName string `json:"client_name,omitempty"`
}

// CLIPairResponse mirrors internal/api's cliPairResponse.
type CLIPairResponse struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"token_id"`
	Username  string    `json:"username"`
	Abilities []string  `json:"abilities"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RedeemCLIPairing calls POST /api/v1/auth/cli-pairing/redeem. Unauthenticated
// by design: build the Client with an empty token, the code is the credential.
func (c *Client) RedeemCLIPairing(ctx context.Context, req CLIPairRequest) (CLIPairResponse, error) {
	var out CLIPairResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/cli-pairing/redeem", req, &out)
	return out, err
}
