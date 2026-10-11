package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// TeamResource mirrors internal/api's teamResource.
type TeamResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Default     bool   `json:"default"`
	Role        string `json:"role"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
}

// TeamMemberResource mirrors internal/api's teamMemberResource.
type TeamMemberResource struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

// TeamRoleResource mirrors internal/api's teamRoleResource.
type TeamRoleResource struct {
	Name      string   `json:"name"`
	Abilities []string `json:"abilities"`
}

// TeamInviteResource mirrors internal/api's teamInviteResource.
type TeamInviteResource struct {
	ID        string `json:"id"`
	TeamID    string `json:"team_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Status    string `json:"status"`
	Token     string `json:"token,omitempty"`
	Link      string `json:"link,omitempty"`
}

// CreateTeamInviteRequest is POST /api/v1/teams/{id}/invites' body.
type CreateTeamInviteRequest struct {
	Email          string `json:"email,omitempty"`
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
}

// AcceptTeamInviteRequest is POST /api/v1/team-invites/accept's body.
type AcceptTeamInviteRequest struct {
	Token       string `json:"token"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Password    string `json:"password,omitempty"`
}

// TeamEventResource mirrors internal/api's teamEventResource.
type TeamEventResource struct {
	ID        string `json:"id"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func teamPath(id string) string { return "/api/v1/teams/" + PathEscape(id) }

// ListTeams calls GET /api/v1/teams.
func (c *Client) ListTeams(ctx context.Context) ([]TeamResource, error) {
	var out []TeamResource
	err := c.do(ctx, http.MethodGet, "/api/v1/teams", nil, &out)
	return out, err
}

// CreateTeam calls POST /api/v1/teams.
func (c *Client) CreateTeam(ctx context.Context, name string) (TeamResource, error) {
	var out TeamResource
	err := c.do(ctx, http.MethodPost, "/api/v1/teams", map[string]string{"name": name}, &out)
	return out, err
}

// GetTeam calls GET /api/v1/teams/{id}.
func (c *Client) GetTeam(ctx context.Context, id string) (TeamResource, error) {
	var out TeamResource
	err := c.do(ctx, http.MethodGet, teamPath(id), nil, &out)
	return out, err
}

// RenameTeam calls PATCH /api/v1/teams/{id}.
func (c *Client) RenameTeam(ctx context.Context, id, name string) (TeamResource, error) {
	var out TeamResource
	err := c.do(ctx, http.MethodPatch, teamPath(id), map[string]string{"name": name}, &out)
	return out, err
}

// DeleteTeam calls DELETE /api/v1/teams/{id}.
func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, teamPath(id), nil, nil)
}

// ListTeamRoles calls GET /api/v1/teams/roles.
func (c *Client) ListTeamRoles(ctx context.Context) ([]TeamRoleResource, error) {
	var out []TeamRoleResource
	err := c.do(ctx, http.MethodGet, "/api/v1/teams/roles", nil, &out)
	return out, err
}

// ListTeamMembers calls GET /api/v1/teams/{id}/members.
func (c *Client) ListTeamMembers(ctx context.Context, id string) ([]TeamMemberResource, error) {
	var out []TeamMemberResource
	err := c.do(ctx, http.MethodGet, teamPath(id)+"/members", nil, &out)
	return out, err
}

// SetTeamMemberRole calls PUT /api/v1/teams/{id}/members/{user}.
func (c *Client) SetTeamMemberRole(ctx context.Context, id, userID, role string) error {
	return c.do(ctx, http.MethodPut, teamPath(id)+"/members/"+PathEscape(userID), map[string]string{"role": role}, nil)
}

// RemoveTeamMember calls DELETE /api/v1/teams/{id}/members/{user}.
func (c *Client) RemoveTeamMember(ctx context.Context, id, userID string) error {
	return c.do(ctx, http.MethodDelete, teamPath(id)+"/members/"+PathEscape(userID), nil, nil)
}

// TransferTeam calls POST /api/v1/teams/{id}/transfer.
func (c *Client) TransferTeam(ctx context.Context, id, userID string) error {
	return c.do(ctx, http.MethodPost, teamPath(id)+"/transfer", map[string]string{"user_id": userID}, nil)
}

// ListTeamInvites calls GET /api/v1/teams/{id}/invites.
func (c *Client) ListTeamInvites(ctx context.Context, id string) ([]TeamInviteResource, error) {
	var out []TeamInviteResource
	err := c.do(ctx, http.MethodGet, teamPath(id)+"/invites", nil, &out)
	return out, err
}

// CreateTeamInvite calls POST /api/v1/teams/{id}/invites.
func (c *Client) CreateTeamInvite(ctx context.Context, id string, req CreateTeamInviteRequest) (TeamInviteResource, error) {
	var out TeamInviteResource
	err := c.do(ctx, http.MethodPost, teamPath(id)+"/invites", req, &out)
	return out, err
}

// RevokeTeamInvite calls DELETE /api/v1/teams/{id}/invites/{invite}.
func (c *Client) RevokeTeamInvite(ctx context.Context, id, inviteID string) error {
	return c.do(ctx, http.MethodDelete, teamPath(id)+"/invites/"+PathEscape(inviteID), nil, nil)
}

// AcceptTeamInvite calls POST /api/v1/team-invites/accept.
func (c *Client) AcceptTeamInvite(ctx context.Context, req AcceptTeamInviteRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/team-invites/accept", req, nil)
}

// ListTeamEvents calls GET /api/v1/teams/{id}/events.
func (c *Client) ListTeamEvents(ctx context.Context, id string, limit int) ([]TeamEventResource, error) {
	var out []TeamEventResource
	err := c.do(ctx, http.MethodGet, teamPath(id)+"/events?limit="+strconv.Itoa(limit), nil, &out)
	return out, err
}
