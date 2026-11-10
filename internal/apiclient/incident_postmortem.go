package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PostmortemAlert is one alert source listed in a postmortem.
type PostmortemAlert struct {
	Kind   string     `json:"kind"`
	Ref    string     `json:"ref"`
	DownAt *time.Time `json:"down_at,omitempty"`
	UpAt   *time.Time `json:"up_at,omitempty"`
}

// IncidentPostmortem mirrors GET /api/v1/incidents/{id}/postmortem.
type IncidentPostmortem struct {
	IncidentID  string            `json:"incident_id"`
	State       string            `json:"state"`
	Summary     string            `json:"summary"`
	Impact      string            `json:"impact"`
	RootCause   string            `json:"root_cause"`
	Resolution  string            `json:"resolution"`
	ActionItems []string          `json:"action_items"`
	ImpactStart time.Time         `json:"impact_start"`
	ImpactEnd   *time.Time        `json:"impact_end,omitempty"`
	Alerts      []PostmortemAlert `json:"alerts"`
	PublishedBy string            `json:"published_by,omitempty"`
	PublishedAt *time.Time        `json:"published_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PostmortemState is what every postmortem write returns.
type PostmortemState struct {
	IncidentID string    `json:"incident_id"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PostmortemEditRequest is the PUT body; a nil field keeps the saved value.
type PostmortemEditRequest struct {
	Summary     *string   `json:"summary,omitempty"`
	Impact      *string   `json:"impact,omitempty"`
	RootCause   *string   `json:"root_cause,omitempty"`
	Resolution  *string   `json:"resolution,omitempty"`
	ActionItems *[]string `json:"action_items,omitempty"`
}

// GetIncidentPostmortem calls GET /api/v1/incidents/{id}/postmortem.
func (c *Client) GetIncidentPostmortem(ctx context.Context, id string) (IncidentPostmortem, error) {
	var out IncidentPostmortem
	err := c.do(ctx, http.MethodGet, incidentPath(id, "/postmortem"), nil, &out)
	return out, err
}

// DraftIncidentPostmortem calls POST /api/v1/incidents/{id}/postmortem/draft.
func (c *Client) DraftIncidentPostmortem(ctx context.Context, id string) (PostmortemState, error) {
	var out PostmortemState
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/postmortem/draft"), nil, &out)
	return out, err
}

// EditIncidentPostmortem calls PUT /api/v1/incidents/{id}/postmortem.
func (c *Client) EditIncidentPostmortem(ctx context.Context, id string, req PostmortemEditRequest) (PostmortemState, error) {
	var out PostmortemState
	err := c.do(ctx, http.MethodPut, incidentPath(id, "/postmortem"), req, &out)
	return out, err
}

// PublishIncidentPostmortem calls POST /api/v1/incidents/{id}/postmortem/publish.
func (c *Client) PublishIncidentPostmortem(ctx context.Context, id string) (PostmortemState, error) {
	var out PostmortemState
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/postmortem/publish"), nil, &out)
	return out, err
}
