package apiclient

import (
	"context"
	"net/http"
)

// DockerGuardRuleCount mirrors one since-boot counter in the guard status.
type DockerGuardRuleCount struct {
	Rule      string `json:"rule"`
	Denied    int64  `json:"denied"`
	WouldDeny int64  `json:"would_deny"`
	LastSeen  string `json:"last_seen"`
	LastPath  string `json:"last_path"`
}

// DockerGuardRuleSummary mirrors one rule's audit history over the window.
type DockerGuardRuleSummary struct {
	Rule      string `json:"rule"`
	Denied    int    `json:"denied"`
	WouldDeny int    `json:"would_deny"`
	LastSeen  string `json:"last_seen"`
	LastPath  string `json:"last_path"`
}

// DockerGuardResource mirrors GET /api/v1/system/docker-guard.
type DockerGuardResource struct {
	Mode            string                   `json:"mode"`
	Source          string                   `json:"source"`
	Effective       string                   `json:"effective"`
	Running         bool                     `json:"running"`
	Socket          string                   `json:"socket,omitempty"`
	Upstream        string                   `json:"upstream,omitempty"`
	RestartRequired bool                     `json:"restart_required"`
	ConfigError     string                   `json:"config_error,omitempty"`
	StartError      string                   `json:"start_error,omitempty"`
	AuditSince      string                   `json:"audit_since,omitempty"`
	SinceBoot       []DockerGuardRuleCount   `json:"since_boot"`
	Rules           []string                 `json:"rules"`
	Configured      bool                     `json:"configured"`
	WindowSeconds   int64                    `json:"window_seconds"`
	Window          []DockerGuardRuleSummary `json:"window"`
	WouldDenyTotal  int                      `json:"would_deny_total"`
	DeniedTotal     int                      `json:"denied_total"`
	ReadyToEnforce  bool                     `json:"ready_to_enforce"`
}

// UpdateDockerGuardRequest is the PUT /api/v1/system/docker-guard body.
type UpdateDockerGuardRequest struct {
	Mode string `json:"mode"`
}

// GetDockerGuard calls GET /api/v1/system/docker-guard.
func (c *Client) GetDockerGuard(ctx context.Context) (DockerGuardResource, error) {
	var out DockerGuardResource
	err := c.do(ctx, http.MethodGet, "/api/v1/system/docker-guard", nil, &out)
	return out, err
}

// UpdateDockerGuard calls PUT /api/v1/system/docker-guard.
func (c *Client) UpdateDockerGuard(ctx context.Context, req UpdateDockerGuardRequest) (DockerGuardResource, error) {
	var out DockerGuardResource
	err := c.do(ctx, http.MethodPut, "/api/v1/system/docker-guard", req, &out)
	return out, err
}
