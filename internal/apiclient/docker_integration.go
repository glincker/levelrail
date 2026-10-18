package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DockerLintFinding mirrors internal/dockerlint.Finding.
type DockerLintFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Fixable  bool   `json:"fixable"`
}

// DockerLintResource is the POST /api/v1/docker/lint response.
type DockerLintResource struct {
	Findings         []DockerLintFinding `json:"findings"`
	FixedDockerfile  string              `json:"fixed_dockerfile"`
	DockerignoreHint string              `json:"dockerignore_suggestion,omitempty"`
}

// LintDockerfile calls POST /api/v1/docker/lint.
func (c *Client) LintDockerfile(ctx context.Context, dockerfile string, hasDockerignore bool) (DockerLintResource, error) {
	var out DockerLintResource
	body := map[string]any{"dockerfile": dockerfile, "has_dockerignore": hasDockerignore}
	err := c.do(ctx, http.MethodPost, "/api/v1/docker/lint", body, &out)
	return out, err
}

// DockerHealthCheck is one row of the Docker health report.
type DockerHealthCheck struct {
	Code      string `json:"code"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	FixAction string `json:"fix_action,omitempty"`
}

// DockerHealthResource is the GET /api/v1/docker/health response.
type DockerHealthResource struct {
	Reachable   bool                `json:"reachable"`
	Version     string              `json:"version,omitempty"`
	Arch        string              `json:"arch,omitempty"`
	OS          string              `json:"os,omitempty"`
	RuntimeKind string              `json:"runtime_kind,omitempty"`
	Rootless    bool                `json:"rootless"`
	DiskTotal   int64               `json:"disk_total_bytes"`
	DiskReclaim int64               `json:"disk_reclaimable_bytes"`
	CheckedAt   time.Time           `json:"checked_at"`
	Checks      []DockerHealthCheck `json:"checks"`
}

// GetDockerHealth calls GET /api/v1/docker/health.
func (c *Client) GetDockerHealth(ctx context.Context) (DockerHealthResource, error) {
	var out DockerHealthResource
	err := c.do(ctx, http.MethodGet, "/api/v1/docker/health", nil, &out)
	return out, err
}

// ImageStatusResource is the GET /api/v1/apps/{name}/image-status response.
type ImageStatusResource struct {
	Image           string `json:"image"`
	Source          string `json:"source"`
	CurrentDigest   string `json:"current_digest,omitempty"`
	LatestDigest    string `json:"latest_digest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ImageArch       string `json:"image_arch,omitempty"`
	NodeArch        string `json:"node_arch,omitempty"`
	ArchMismatch    bool   `json:"arch_mismatch"`
	Message         string `json:"message,omitempty"`
}

// GetImageStatus calls GET /api/v1/apps/{name}/image-status.
func (c *Client) GetImageStatus(ctx context.Context, appName string) (ImageStatusResource, error) {
	var out ImageStatusResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/image-status", nil, &out)
	return out, err
}
