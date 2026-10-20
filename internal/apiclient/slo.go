package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// AlertSLOConfig mirrors internal/alerting.SLOConfig: the request-based SLO a
// kind=slo_burn rule watches. Target is a percentage such as 99.9.
type AlertSLOConfig struct {
	Objective string  `json:"objective"`
	Target    float64 `json:"target"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
}

// SLOWindowStat mirrors internal/api's sloWindowResource.
type SLOWindowStat struct {
	WindowSeconds int64   `json:"window_seconds"`
	Requests      float64 `json:"requests"`
	Bad           float64 `json:"bad"`
	BurnRate      float64 `json:"burn_rate"`
}

// SLOTierStatus mirrors internal/api's sloTierResource.
type SLOTierStatus struct {
	Name          string  `json:"name"`
	Factor        float64 `json:"factor"`
	LongSeconds   int64   `json:"long_seconds"`
	ShortSeconds  int64   `json:"short_seconds"`
	Page          bool    `json:"page"`
	LongBurn      float64 `json:"long_burn"`
	ShortBurn     float64 `json:"short_burn"`
	EffectiveBurn float64 `json:"effective_burn"`
	Firing        bool    `json:"firing"`
}

// SLOPreviewResource mirrors internal/api's sloPreviewResource: the error
// budget left and current burn rates for an SLO.
type SLOPreviewResource struct {
	Config          AlertSLOConfig  `json:"config"`
	HasTraffic      bool            `json:"has_traffic"`
	BudgetRemaining float64         `json:"budget_remaining"`
	BudgetRequests  float64         `json:"budget_window_requests"`
	Windows         []SLOWindowStat `json:"windows"`
	Tiers           []SLOTierStatus `json:"tiers"`
	Firing          bool            `json:"firing"`
	Page            bool            `json:"page"`
	MaxBurn         float64         `json:"max_burn"`
}

// GetSLOPreview calls GET /api/v1/apps/{name}/slo-preview.
func (c *Client) GetSLOPreview(ctx context.Context, name string, cfg AlertSLOConfig) (*SLOPreviewResource, error) {
	v := url.Values{}
	if cfg.Objective != "" {
		v.Set("objective", cfg.Objective)
	}
	if cfg.Target != 0 {
		v.Set("target", strconv.FormatFloat(cfg.Target, 'f', -1, 64))
	}
	if cfg.LatencyMs != 0 {
		v.Set("latency_ms", strconv.FormatFloat(cfg.LatencyMs, 'f', -1, 64))
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/slo-preview"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out SLOPreviewResource
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
