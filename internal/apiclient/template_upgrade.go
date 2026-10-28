package apiclient

import (
	"context"
	"net/http"
)

// TemplateUpgradePlan mirrors GET /api/v1/apps/{name}/template-upgrade.
type TemplateUpgradePlan struct {
	AppID       string                `json:"app_id"`
	TemplateID  string                `json:"template_id"`
	FromVersion string                `json:"from_version"`
	ToVersion   string                `json:"to_version"`
	Risk        string                `json:"risk"`
	HasChanges  bool                  `json:"has_changes"`
	Automatic   bool                  `json:"automatic"`
	Blockers    []TemplateUpgradeStop `json:"blockers"`
	Services    []TemplateUpgradeSvc  `json:"services"`
}

// TemplateUpgradeStop is one reason an upgrade cannot run unattended.
type TemplateUpgradeStop struct {
	Code    string `json:"code"`
	Service string `json:"service"`
	Detail  string `json:"detail"`
}

// TemplateUpgradeSvc is one service's change set; names and tags only.
type TemplateUpgradeSvc struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Image  *struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Kind      string `json:"kind"`
		Preserved bool   `json:"preserved"`
	} `json:"image,omitempty"`
	Env []struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	} `json:"env,omitempty"`
	Volumes []struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	} `json:"volumes,omitempty"`
	Ports []struct {
		Port   int    `json:"port"`
		Action string `json:"action"`
	} `json:"ports,omitempty"`
	Healthcheck *struct {
		Preserved bool `json:"preserved"`
	} `json:"healthcheck,omitempty"`
}

// GetTemplateUpgradePlan calls GET /api/v1/apps/{name}/template-upgrade.
func (c *Client) GetTemplateUpgradePlan(ctx context.Context, name string) (TemplateUpgradePlan, error) {
	var out TemplateUpgradePlan
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/template-upgrade", nil, &out)
	return out, err
}
