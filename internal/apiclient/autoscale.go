package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/autoscale"
)

// AppAutoscale is GET/PUT /api/v1/apps/{name}/autoscale.
type AppAutoscale = autoscale.Status

// AppAutoscaleUpdate is the PUT body; it replaces the whole policy. Unset
// tolerance, cooldown and step fields take their defaults.
type AppAutoscaleUpdate struct {
	MinReplicas           int     `json:"min_replicas"`
	MaxReplicas           int     `json:"max_replicas"`
	Metric                string  `json:"metric"`
	Target                float64 `json:"target"`
	ScaleUpTolerancePct   int     `json:"scale_up_tolerance_pct,omitempty"`
	ScaleDownTolerancePct int     `json:"scale_down_tolerance_pct,omitempty"`
	ScaleUpCooldownSecs   int     `json:"scale_up_cooldown_secs,omitempty"`
	ScaleDownCooldownSecs int     `json:"scale_down_cooldown_secs,omitempty"`
	StabilizationSecs     int     `json:"stabilization_secs,omitempty"`
	MaxStepUp             int     `json:"max_step_up,omitempty"`
	MaxStepDown           int     `json:"max_step_down,omitempty"`
}

// AppAutoscaleHistory is GET /api/v1/apps/{name}/autoscale/history.
type AppAutoscaleHistory struct {
	Events []autoscale.Event `json:"events"`
}

func appAutoscalePath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/autoscale"
}

// GetAppAutoscale calls GET /api/v1/apps/{name}/autoscale; an app with no
// policy fails with an APIError of status 404.
func (c *Client) GetAppAutoscale(ctx context.Context, app string) (AppAutoscale, error) {
	var out AppAutoscale
	err := c.do(ctx, http.MethodGet, appAutoscalePath(app), nil, &out)
	return out, err
}

// SetAppAutoscale calls PUT /api/v1/apps/{name}/autoscale.
func (c *Client) SetAppAutoscale(ctx context.Context, app string, u AppAutoscaleUpdate) (AppAutoscale, error) {
	var out AppAutoscale
	err := c.do(ctx, http.MethodPut, appAutoscalePath(app), u, &out)
	return out, err
}

// ClearAppAutoscale calls DELETE /api/v1/apps/{name}/autoscale.
func (c *Client) ClearAppAutoscale(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, appAutoscalePath(app), nil, nil)
}

// GetAppAutoscaleHistory calls GET /api/v1/apps/{name}/autoscale/history.
func (c *Client) GetAppAutoscaleHistory(ctx context.Context, app string) (AppAutoscaleHistory, error) {
	var out AppAutoscaleHistory
	err := c.do(ctx, http.MethodGet, appAutoscalePath(app)+"/history", nil, &out)
	return out, err
}
