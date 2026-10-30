package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// IncidentTimelineEntry is one internal or public timeline row.
type IncidentTimelineEntry struct {
	Status     string    `json:"status"`
	Body       string    `json:"body"`
	Visibility string    `json:"visibility"`
	Author     string    `json:"author,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// IncidentDraft is the unpublished public text.
type IncidentDraft struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Incident is an internal incident as the control plane returns it.
type Incident struct {
	ID             string                  `json:"id"`
	Title          string                  `json:"title"`
	Status         string                  `json:"status"`
	Impact         string                  `json:"impact"`
	Source         string                  `json:"source"`
	SourceRef      string                  `json:"source_ref,omitempty"`
	InternalState  string                  `json:"internal_state"`
	Published      bool                    `json:"published"`
	PublishedAt    *time.Time              `json:"published_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at"`
	Draft          *IncidentDraft          `json:"draft,omitempty"`
	ComponentIDs   []string                `json:"component_ids"`
	StartsAt       time.Time               `json:"starts_at"`
	AcknowledgedAt *time.Time              `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string                  `json:"acknowledged_by,omitempty"`
	ResolvedAt     *time.Time              `json:"internal_resolved_at,omitempty"`
	Timeline       []IncidentTimelineEntry `json:"timeline,omitempty"`
}

// IncidentState is what an incident mutation returns.
type IncidentState struct {
	ID            string `json:"id"`
	InternalState string `json:"internal_state"`
}

// IncidentDraftRequest is the body of PUT .../draft. Nil ComponentIDs keeps the stored list.
type IncidentDraftRequest struct {
	Title        string    `json:"title"`
	Status       string    `json:"status,omitempty"`
	Body         string    `json:"body"`
	Impact       string    `json:"impact,omitempty"`
	ComponentIDs *[]string `json:"component_ids,omitempty"`
}

// IncidentPublished is the publish result.
type IncidentPublished struct {
	ID          string    `json:"id"`
	Published   bool      `json:"published"`
	PublishedAt time.Time `json:"published_at"`
}

func incidentPath(id string, rest string) string {
	return "/api/v1/incidents/" + PathEscape(id) + rest
}

// ListIncidents calls GET /api/v1/incidents, optionally filtered by internal state.
func (c *Client) ListIncidents(ctx context.Context, state string) ([]Incident, error) {
	path := "/api/v1/incidents"
	if state != "" {
		path += "?state=" + url.QueryEscape(state)
	}
	var out []Incident
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetIncident calls GET /api/v1/incidents/{id}.
func (c *Client) GetIncident(ctx context.Context, id string) (Incident, error) {
	var out Incident
	err := c.do(ctx, http.MethodGet, incidentPath(id, ""), nil, &out)
	return out, err
}

// PromoteIncident calls POST /api/v1/incidents for a firing alert rule.
func (c *Client) PromoteIncident(ctx context.Context, ruleID, title string) (IncidentState, error) {
	var out IncidentState
	body := map[string]string{"source": "alert", "rule_id": ruleID, "title": title}
	err := c.do(ctx, http.MethodPost, "/api/v1/incidents", body, &out)
	return out, err
}

// AckIncident calls POST /api/v1/incidents/{id}/ack.
func (c *Client) AckIncident(ctx context.Context, id string) (IncidentState, error) {
	var out IncidentState
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/ack"), nil, &out)
	return out, err
}

// ResolveIncident calls POST /api/v1/incidents/{id}/resolve.
func (c *Client) ResolveIncident(ctx context.Context, id string) (IncidentState, error) {
	var out IncidentState
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/resolve"), nil, &out)
	return out, err
}

// AddIncidentNote calls POST /api/v1/incidents/{id}/notes (internal only).
func (c *Client) AddIncidentNote(ctx context.Context, id, body string) (IncidentState, error) {
	var out IncidentState
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/notes"), map[string]string{"body": body}, &out)
	return out, err
}

// SaveIncidentDraft calls PUT /api/v1/incidents/{id}/draft.
func (c *Client) SaveIncidentDraft(ctx context.Context, id string, req IncidentDraftRequest) (IncidentState, error) {
	var out IncidentState
	err := c.do(ctx, http.MethodPut, incidentPath(id, "/draft"), req, &out)
	return out, err
}

// PublishIncident calls POST /api/v1/incidents/{id}/publish with the updated_at the operator reviewed.
func (c *Client) PublishIncident(ctx context.Context, id string, reviewed time.Time) (IncidentPublished, error) {
	var out IncidentPublished
	err := c.do(ctx, http.MethodPost, incidentPath(id, "/publish"), map[string]time.Time{"updated_at": reviewed}, &out)
	return out, err
}
