package apiclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// OrganizationProfile mirrors the organization resource with its profile fields.
type OrganizationProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CreatedAt   string `json:"created_at"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// OrganizationEvent is one activity item of an organization.
type OrganizationEvent struct {
	ID        string `json:"id"`
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// OrganizationSummary mirrors GET /api/v1/organizations/{id}/summary.
type OrganizationSummary struct {
	Organization OrganizationProfile `json:"organization"`
	Counts       struct {
		Projects       int `json:"projects"`
		Apps           int `json:"apps"`
		Databases      int `json:"databases"`
		Environments   int `json:"environments"`
		Members        int `json:"members"`
		PendingInvites int `json:"pending_invites"`
	} `json:"counts"`
	FailingApps struct {
		Count int      `json:"count"`
		Names []string `json:"names"`
	} `json:"failing_apps"`
	Activity []OrganizationEvent `json:"activity"`
}

// GetOrganizationSummary calls GET /api/v1/organizations/{id}/summary.
func (c *Client) GetOrganizationSummary(ctx context.Context, id string) (OrganizationSummary, error) {
	var out OrganizationSummary
	err := c.do(ctx, http.MethodGet, organizationPath(id)+"/summary", nil, &out)
	return out, err
}

// PatchOrganization calls PATCH /api/v1/organizations/{id} with the given fields only.
func (c *Client) PatchOrganization(ctx context.Context, id string, fields map[string]string) (OrganizationProfile, error) {
	var out OrganizationProfile
	err := c.do(ctx, http.MethodPatch, organizationPath(id), fields, &out)
	return out, err
}

// SetOrganizationAvatar calls PUT /api/v1/organizations/{id}/avatar with the raw image.
func (c *Client) SetOrganizationAvatar(ctx context.Context, id string, image []byte) (OrganizationProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+organizationPath(id)+"/avatar", bytes.NewReader(image)) //nolint:gosec // operator-supplied API target
	if err != nil {
		return OrganizationProfile{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(headerContentType, "application/octet-stream")
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return OrganizationProfile{}, fmt.Errorf("request PUT %s: %w%s", organizationPath(id)+"/avatar", err, connectHint(err))
	}
	defer func() { _ = resp.Body.Close() }()
	var out OrganizationProfile
	return out, decodeResponse(resp, &out)
}

// DeleteOrganizationAvatar calls DELETE /api/v1/organizations/{id}/avatar.
func (c *Client) DeleteOrganizationAvatar(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, organizationPath(id)+"/avatar", nil, nil)
}
