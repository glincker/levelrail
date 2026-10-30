package apiclient

import (
	"context"
	"net/http"
)

type tokenCleanupRequest struct {
	OlderThanDays int  `json:"older_than_days"`
	Confirm       bool `json:"confirm,omitempty"`
}

// TokenCleanupResult is the body of POST /api/v1/auth/tokens/cleanup.
type TokenCleanupResult struct {
	Deleted int `json:"deleted"`
}

// CleanupTokens calls POST /api/v1/auth/tokens/cleanup (root ability). It
// removes revoked tokens older than olderThanDays; 0 removes every revoked token and is confirmed here.
func (c *Client) CleanupTokens(ctx context.Context, olderThanDays int) (TokenCleanupResult, error) {
	var out TokenCleanupResult
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/tokens/cleanup", tokenCleanupRequest{OlderThanDays: olderThanDays, Confirm: olderThanDays == 0}, &out)
	return out, err
}
