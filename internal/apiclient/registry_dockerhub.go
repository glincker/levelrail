package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DockerHubLimitsResource mirrors the Docker Hub pull limit wire shape.
type DockerHubLimitsResource struct {
	CredentialID  string    `json:"credential_id,omitempty"`
	Authenticated bool      `json:"authenticated"`
	Known         bool      `json:"known"`
	Limit         int       `json:"limit"`
	Remaining     int       `json:"remaining"`
	WindowSeconds int       `json:"window_seconds"`
	Source        string    `json:"source"`
	Low           bool      `json:"low"`
	WarnPercent   int       `json:"warn_percent"`
	UseForPulls   bool      `json:"use_for_pulls"`
	CheckedAt     time.Time `json:"checked_at"`
	Cached        bool      `json:"cached"`
}

// DockerHubCapabilityResource is one checked Docker Hub capability.
type DockerHubCapabilityResource struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// DockerHubStatusResource mirrors GET /api/v1/registry-credentials/{id}/dockerhub.
type DockerHubStatusResource struct {
	CredentialID        string                        `json:"credential_id,omitempty"`
	Valid               bool                          `json:"valid"`
	Kind                string                        `json:"kind"`
	Namespace           string                        `json:"namespace"`
	Message             string                        `json:"message,omitempty"`
	Scopes              []string                      `json:"scopes"`
	Capabilities        []DockerHubCapabilityResource `json:"capabilities"`
	PrivateReposVisible bool                          `json:"private_repos_visible"`
	ExpiryKnown         bool                          `json:"expiry_known"`
	ExpiresAt           *time.Time                    `json:"expires_at"`
	PermissionNote      string                        `json:"permission_note"`
	Limits              *DockerHubLimitsResource      `json:"limits"`
	CheckedAt           time.Time                     `json:"checked_at"`
	Cached              bool                          `json:"cached"`
}

// GetDockerHubStatus calls GET /api/v1/registry-credentials/{id}/dockerhub.
func (c *Client) GetDockerHubStatus(ctx context.Context, id string, refresh bool) (DockerHubStatusResource, error) {
	path := registryCredentialPath(id) + "/dockerhub"
	if refresh {
		path += "?refresh=true"
	}
	var out DockerHubStatusResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetDockerHubLimits calls GET /api/v1/registry-credentials/{id}/limits.
func (c *Client) GetDockerHubLimits(ctx context.Context, id string, refresh bool) (DockerHubLimitsResource, error) {
	path := registryCredentialPath(id) + "/limits"
	if refresh {
		path += "?refresh=true"
	}
	var out DockerHubLimitsResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
