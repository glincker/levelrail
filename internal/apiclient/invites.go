package apiclient

import (
	"context"
	"net/http"
	"time"
)

// InviteResource mirrors internal/api's inviteResource (internal/api/invites.go).
type InviteResource struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role,omitempty"`
	Abilities []string  `json:"abilities"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Expired   bool      `json:"expired"`
}

// CreateInviteRequest mirrors internal/api's createInviteRequest. Role,
// when set, takes precedence server-side over Abilities, same precedence
// CreateUserRequest documents.
type CreateInviteRequest struct {
	Email     string   `json:"email"`
	Role      string   `json:"role,omitempty"`
	Abilities []string `json:"abilities,omitempty"`
}

// CreateInviteResponse mirrors internal/api's createInviteResponse: the
// invite plus Link, the accept URL to hand or send to the invited person.
type CreateInviteResponse struct {
	InviteResource
	Link string `json:"link"`
}

// CreateInvite calls POST /api/v1/invites.
func (c *Client) CreateInvite(ctx context.Context, req CreateInviteRequest) (CreateInviteResponse, error) {
	var out CreateInviteResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/invites", req, &out)
	return out, err
}

// ListInvites calls GET /api/v1/invites.
func (c *Client) ListInvites(ctx context.Context) ([]InviteResource, error) {
	return getJSON[[]InviteResource](ctx, c, "/api/v1/invites")
}

// RevokeInvite calls DELETE /api/v1/invites/{id}.
func (c *Client) RevokeInvite(ctx context.Context, id string) error {
	return c.del(ctx, "/api/v1/invites/"+PathEscape(id))
}
