package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SilenceMatchers mirrors alerting.SilenceMatcher.
type SilenceMatchers struct {
	RuleIDs    []string          `json:"rule_ids,omitempty"`
	Kinds      []string          `json:"kinds,omitempty"`
	Apps       []string          `json:"apps,omitempty"`
	Nodes      []string          `json:"nodes,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Severities []string          `json:"severities,omitempty"`
}

// SilenceResource mirrors internal/api's silenceResource.
type SilenceResource struct {
	ID        string          `json:"id"`
	Matchers  SilenceMatchers `json:"matchers"`
	StartsAt  time.Time       `json:"starts_at"`
	EndsAt    time.Time       `json:"ends_at"`
	CreatedBy string          `json:"created_by"`
	Reason    string          `json:"reason,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiredAt *time.Time      `json:"expired_at,omitempty"`
	Status    string          `json:"status"`
}

// CreateSilenceRequest is the body of POST /api/v1/alert-silences. Set
// Duration or EndsAt; StartsAt defaults to now.
type CreateSilenceRequest struct {
	Matchers SilenceMatchers `json:"matchers"`
	StartsAt *time.Time      `json:"starts_at,omitempty"`
	EndsAt   *time.Time      `json:"ends_at,omitempty"`
	Duration string          `json:"duration,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// MaintenanceWindowResource mirrors internal/api's maintenanceWindowResource.
type MaintenanceWindowResource struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Cron        string     `json:"cron"`
	Duration    string     `json:"duration"`
	Timezone    string     `json:"timezone,omitempty"`
	Scope       string     `json:"scope"`
	Targets     []string   `json:"targets,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Active      bool       `json:"active"`
	ActiveUntil *time.Time `json:"active_until,omitempty"`
	NextStart   *time.Time `json:"next_start,omitempty"`
}

// AlertHistoryEntry mirrors internal/api's alertHistoryResource.
type AlertHistoryEntry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	RuleKind   string    `json:"rule_kind"`
	ResourceID string    `json:"resource_id,omitempty"`
	App        string    `json:"app,omitempty"`
	Node       string    `json:"node,omitempty"`
	Severity   string    `json:"severity,omitempty"`
	Event      string    `json:"event"`
	Outcome    string    `json:"outcome"`
	Detail     string    `json:"detail,omitempty"`
	SilenceID  string    `json:"silence_id,omitempty"`
	ChannelID  string    `json:"channel_id,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// AlertHistoryQuery filters ListAlertHistory; zero fields are omitted.
type AlertHistoryQuery struct {
	App     string
	RuleID  string
	Outcome string
	Event   string
	Since   string
	Limit   int
}

func (q AlertHistoryQuery) encode() string {
	v := url.Values{}
	for k, val := range map[string]string{"app": q.App, "rule_id": q.RuleID, "outcome": q.Outcome, "event": q.Event, "since": q.Since} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// ListAlertSilences calls GET /api/v1/alert-silences.
func (c *Client) ListAlertSilences(ctx context.Context, includeExpired bool) ([]SilenceResource, error) {
	path := "/api/v1/alert-silences"
	if includeExpired {
		path += "?include_expired=true"
	}
	var out []SilenceResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// CreateAlertSilence calls POST /api/v1/alert-silences.
func (c *Client) CreateAlertSilence(ctx context.Context, req CreateSilenceRequest) (SilenceResource, error) {
	var out SilenceResource
	err := c.do(ctx, http.MethodPost, "/api/v1/alert-silences", req, &out)
	return out, err
}

// ExpireAlertSilence calls DELETE /api/v1/alert-silences/{id}, ending it now.
func (c *Client) ExpireAlertSilence(ctx context.Context, id string) (SilenceResource, error) {
	var out SilenceResource
	err := c.do(ctx, http.MethodDelete, "/api/v1/alert-silences/"+PathEscape(id), nil, &out)
	return out, err
}

// SilenceAlertRule calls POST /api/v1/apps/{name}/alerts/{id}/silence.
func (c *Client) SilenceAlertRule(ctx context.Context, app, ruleID, duration, reason string) (SilenceResource, error) {
	var out SilenceResource
	body := CreateSilenceRequest{Duration: duration, Reason: reason}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/alerts/"+PathEscape(ruleID)+"/silence", body, &out)
	return out, err
}

// ListMaintenanceWindows calls GET /api/v1/alert-maintenance-windows.
func (c *Client) ListMaintenanceWindows(ctx context.Context) ([]MaintenanceWindowResource, error) {
	var out []MaintenanceWindowResource
	err := c.do(ctx, http.MethodGet, "/api/v1/alert-maintenance-windows", nil, &out)
	return out, err
}

// CreateMaintenanceWindow calls POST /api/v1/alert-maintenance-windows.
func (c *Client) CreateMaintenanceWindow(ctx context.Context, req MaintenanceWindowResource) (MaintenanceWindowResource, error) {
	var out MaintenanceWindowResource
	err := c.do(ctx, http.MethodPost, "/api/v1/alert-maintenance-windows", req, &out)
	return out, err
}

// UpdateMaintenanceWindow calls PUT /api/v1/alert-maintenance-windows/{id}.
func (c *Client) UpdateMaintenanceWindow(ctx context.Context, id string, req MaintenanceWindowResource) (MaintenanceWindowResource, error) {
	var out MaintenanceWindowResource
	err := c.do(ctx, http.MethodPut, "/api/v1/alert-maintenance-windows/"+PathEscape(id), req, &out)
	return out, err
}

// DeleteMaintenanceWindow calls DELETE /api/v1/alert-maintenance-windows/{id}.
func (c *Client) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/alert-maintenance-windows/"+PathEscape(id), nil, nil)
}

// ListAlertHistory calls GET /api/v1/alert-history.
func (c *Client) ListAlertHistory(ctx context.Context, q AlertHistoryQuery) ([]AlertHistoryEntry, error) {
	var out []AlertHistoryEntry
	err := c.do(ctx, http.MethodGet, "/api/v1/alert-history"+q.encode(), nil, &out)
	return out, err
}

// StatusPageSettings mirrors internal/api's statusSettingsResource.
type StatusPageSettings struct {
	Enabled      bool   `json:"enabled"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CustomDomain string `json:"custom_domain"`
	PublicPath   string `json:"public_path,omitempty"`
}

// StatusComponent mirrors internal/api's statusComponentResource.
type StatusComponent struct {
	ID          string `json:"id,omitempty"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	DisplayName string `json:"display_name"`
	Position    int    `json:"position"`
}

// StatusUpdate is one incident timeline entry.
type StatusUpdate struct {
	Status    string    `json:"status"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// StatusIncident mirrors internal/api's statusIncidentResource.
type StatusIncident struct {
	ID           string         `json:"id,omitempty"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	Status       string         `json:"status"`
	Impact       string         `json:"impact"`
	ComponentIDs []string       `json:"component_ids"`
	StartsAt     *time.Time     `json:"starts_at,omitempty"`
	EndsAt       *time.Time     `json:"ends_at,omitempty"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
	Body         string         `json:"body,omitempty"`
	Updates      []StatusUpdate `json:"updates,omitempty"`
}

// StatusPageComponentView is one component in the preview endpoint's view.
type StatusPageComponentView struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Uptime90 *float64 `json:"uptime_90d"`
}

// StatusPageView is the public payload as the preview endpoint returns it.
type StatusPageView struct {
	Title      string                    `json:"title"`
	Status     string                    `json:"status"`
	StatusText string                    `json:"status_text"`
	Components []StatusPageComponentView `json:"components"`
}

// GetStatusPage calls GET /api/v1/status-page.
func (c *Client) GetStatusPage(ctx context.Context) (StatusPageSettings, error) {
	var out StatusPageSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/status-page", nil, &out)
	return out, err
}

// PutStatusPage calls PUT /api/v1/status-page.
func (c *Client) PutStatusPage(ctx context.Context, s StatusPageSettings) (StatusPageSettings, error) {
	var out StatusPageSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/status-page", s, &out)
	return out, err
}

// StatusPagePreview calls GET /api/v1/status-page/preview.
func (c *Client) StatusPagePreview(ctx context.Context) (StatusPageView, error) {
	var out StatusPageView
	err := c.do(ctx, http.MethodGet, "/api/v1/status-page/preview", nil, &out)
	return out, err
}

// ListStatusComponents calls GET /api/v1/status-page/components.
func (c *Client) ListStatusComponents(ctx context.Context) ([]StatusComponent, error) {
	var out []StatusComponent
	err := c.do(ctx, http.MethodGet, "/api/v1/status-page/components", nil, &out)
	return out, err
}

// CreateStatusComponent calls POST /api/v1/status-page/components.
func (c *Client) CreateStatusComponent(ctx context.Context, comp StatusComponent) (StatusComponent, error) {
	var out StatusComponent
	err := c.do(ctx, http.MethodPost, "/api/v1/status-page/components", comp, &out)
	return out, err
}

// DeleteStatusComponent calls DELETE /api/v1/status-page/components/{id}.
func (c *Client) DeleteStatusComponent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/status-page/components/"+PathEscape(id), nil, nil)
}

// ListStatusIncidents calls GET /api/v1/status-page/incidents.
func (c *Client) ListStatusIncidents(ctx context.Context) ([]StatusIncident, error) {
	var out []StatusIncident
	err := c.do(ctx, http.MethodGet, "/api/v1/status-page/incidents", nil, &out)
	return out, err
}

// CreateStatusIncident calls POST /api/v1/status-page/incidents.
func (c *Client) CreateStatusIncident(ctx context.Context, inc StatusIncident) (StatusIncident, error) {
	var out StatusIncident
	err := c.do(ctx, http.MethodPost, "/api/v1/status-page/incidents", inc, &out)
	return out, err
}

// PostStatusIncidentUpdate calls POST /api/v1/status-page/incidents/{id}/updates.
func (c *Client) PostStatusIncidentUpdate(ctx context.Context, id string, u StatusUpdate) (StatusIncident, error) {
	var out StatusIncident
	err := c.do(ctx, http.MethodPost, "/api/v1/status-page/incidents/"+PathEscape(id)+"/updates", u, &out)
	return out, err
}

// DeleteStatusIncident calls DELETE /api/v1/status-page/incidents/{id}.
func (c *Client) DeleteStatusIncident(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/status-page/incidents/"+PathEscape(id), nil, nil)
}
