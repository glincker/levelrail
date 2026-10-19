package apiclient

import (
	"context"
	"net/http"
)

// AlertTemplateRule is one rule a template profile creates.
type AlertTemplateRule struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// AlertTemplateProfile is a one-click rule set for an app shape.
type AlertTemplateProfile struct {
	Profile string              `json:"profile"`
	Rules   []AlertTemplateRule `json:"rules"`
}

// ApplyAlertTemplateResult lists the rules created and the names skipped as already present.
type ApplyAlertTemplateResult struct {
	Profile string              `json:"profile"`
	Created []AlertRuleResource `json:"created"`
	Skipped []string            `json:"skipped"`
}

// QuietHoursResource is a channel's quiet-hours window.
type QuietHoursResource struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
	Enabled  bool   `json:"enabled"`
}

// ListAlertTemplates calls GET /api/v1/alert-templates.
func (c *Client) ListAlertTemplates(ctx context.Context) ([]AlertTemplateProfile, error) {
	var out []AlertTemplateProfile
	if err := c.do(ctx, http.MethodGet, "/api/v1/alert-templates", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApplyAlertTemplate calls POST /api/v1/apps/{name}/alerts/apply-template.
func (c *Client) ApplyAlertTemplate(ctx context.Context, name, profile, channelID string) (ApplyAlertTemplateResult, error) {
	body := map[string]string{"profile": profile, "channel_id": channelID}
	var out ApplyAlertTemplateResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/alerts/apply-template", body, &out)
	return out, err
}

// GetChannelQuietHours calls GET /api/v1/notification-channels/{id}/quiet-hours.
func (c *Client) GetChannelQuietHours(ctx context.Context, id string) (QuietHoursResource, error) {
	var out QuietHoursResource
	err := c.do(ctx, http.MethodGet, "/api/v1/notification-channels/"+PathEscape(id)+"/quiet-hours", nil, &out)
	return out, err
}

// SetChannelQuietHours calls PUT /api/v1/notification-channels/{id}/quiet-hours.
func (c *Client) SetChannelQuietHours(ctx context.Context, id string, q QuietHoursResource) (QuietHoursResource, error) {
	var out QuietHoursResource
	err := c.do(ctx, http.MethodPut, "/api/v1/notification-channels/"+PathEscape(id)+"/quiet-hours", q, &out)
	return out, err
}

// ClearChannelQuietHours calls DELETE /api/v1/notification-channels/{id}/quiet-hours.
func (c *Client) ClearChannelQuietHours(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/notification-channels/"+PathEscape(id)+"/quiet-hours", nil, nil)
}
