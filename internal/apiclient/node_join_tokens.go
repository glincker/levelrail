package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// PendingJoinToken is an unused, unexpired node join token (never its secret).
type PendingJoinToken struct {
	ID        string    `json:"id"`
	Purpose   string    `json:"purpose"`
	NodeID    string    `json:"node_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RevokeJoinTokensResponse reports how many pending tokens were revoked.
type RevokeJoinTokensResponse struct {
	Revoked int64 `json:"revoked"`
}

// ListPendingJoinTokens calls GET /api/v1/nodes/join-tokens.
func (c *Client) ListPendingJoinTokens(ctx context.Context) ([]PendingJoinToken, error) {
	var out []PendingJoinToken
	err := c.do(ctx, http.MethodGet, nodesCollectionPath()+"/join-tokens", nil, &out)
	return out, err
}

// RevokeJoinTokens calls DELETE /api/v1/nodes/join-tokens, or .../{id} when id is set.
func (c *Client) RevokeJoinTokens(ctx context.Context, id string) (RevokeJoinTokensResponse, error) {
	var out RevokeJoinTokensResponse
	p := nodesCollectionPath() + "/join-tokens"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	err := c.do(ctx, http.MethodDelete, p, nil, &out)
	return out, err
}
