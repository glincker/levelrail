package apiclient

import (
	"context"
	"net/http"
	"time"
)

// ScheduledMaintenanceResource mirrors internal/api's scheduledMaintenanceResource.
type ScheduledMaintenanceResource struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Message          string     `json:"message"`
	Scope            string     `json:"scope"`
	Target           string     `json:"target,omitempty"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	Freeze           bool       `json:"freeze"`
	AnnounceLeadMins int        `json:"announce_lead_minutes"`
	AnnouncedAt      *time.Time `json:"announced_at,omitempty"`
	State            string     `json:"state"`
	CreatedBy        string     `json:"created_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ScheduledMaintenanceRequest is the body of POST /api/v1/scheduled-maintenance.
type ScheduledMaintenanceRequest struct {
	Title            string    `json:"title"`
	Message          string    `json:"message,omitempty"`
	Scope            string    `json:"scope"`
	Target           string    `json:"target,omitempty"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	Freeze           bool      `json:"freeze"`
	AnnounceLeadMins int       `json:"announce_lead_minutes"`
}

// ListScheduledMaintenance calls GET /api/v1/scheduled-maintenance.
func (c *Client) ListScheduledMaintenance(ctx context.Context) ([]ScheduledMaintenanceResource, error) {
	var out []ScheduledMaintenanceResource
	err := c.do(ctx, http.MethodGet, "/api/v1/scheduled-maintenance", nil, &out)
	return out, err
}

// CreateScheduledMaintenance calls POST /api/v1/scheduled-maintenance.
func (c *Client) CreateScheduledMaintenance(ctx context.Context, req ScheduledMaintenanceRequest) (ScheduledMaintenanceResource, error) {
	var out ScheduledMaintenanceResource
	err := c.do(ctx, http.MethodPost, "/api/v1/scheduled-maintenance", req, &out)
	return out, err
}

// DeleteScheduledMaintenance calls DELETE /api/v1/scheduled-maintenance/{id}.
func (c *Client) DeleteScheduledMaintenance(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/scheduled-maintenance/"+PathEscape(id), nil, nil)
}

// ListFleetMaintenance calls GET /api/v1/maintenance/domains.
func (c *Client) ListFleetMaintenance(ctx context.Context) (FleetMaintenanceResource, error) {
	var out FleetMaintenanceResource
	err := c.do(ctx, http.MethodGet, "/api/v1/maintenance/domains", nil, &out)
	return out, err
}

// PreviewMaintenancePage calls POST /api/v1/maintenance/preview.
func (c *Client) PreviewMaintenancePage(ctx context.Context, title, message string, retryAfterSeconds int) (MaintenancePreviewResource, error) {
	var out MaintenancePreviewResource
	body := map[string]any{"title": title, "message": message, "retry_after_seconds": retryAfterSeconds}
	err := c.do(ctx, http.MethodPost, "/api/v1/maintenance/preview", body, &out)
	return out, err
}
