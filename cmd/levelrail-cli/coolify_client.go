package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Partial mirror of Coolify's Application schema: only the fields the mapping reads.
type coolifyApplication struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	FQDN                string `json:"fqdn"`
	BuildPack           string `json:"build_pack"`
	DockerfileLocation  string `json:"dockerfile_location"`
	PortsExposes        string `json:"ports_exposes"`
	GitRepository       string `json:"git_repository"`
	GitBranch           string `json:"git_branch"`
	HealthCheckEnabled  bool   `json:"health_check_enabled"`
	HealthCheckPath     string `json:"health_check_path"`
	HealthCheckType     string `json:"health_check_type"`
	HealthCheckCommand  string `json:"health_check_command"`
	HealthCheckInterval int    `json:"health_check_interval"`
	HealthCheckTimeout  int    `json:"health_check_timeout"`
	HealthCheckRetries  int    `json:"health_check_retries"`
	LimitsMemory        string `json:"limits_memory"`
	LimitsCPUs          string `json:"limits_cpus"`
}

// Value is redacted without elevated ability; RealValue is only trusted when real values were requested.
type coolifyEnvVar struct {
	UUID      string `json:"uuid"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	RealValue string `json:"real_value"`
}

type coolifyErrorBody struct {
	Message string `json:"message"`
}

// A non-2xx reply from Coolify, distinct from a network failure.
type coolifyAPIError struct {
	StatusCode int
	Message    string
}

func (e *coolifyAPIError) Error() string {
	return fmt.Sprintf("coolify api returned %d: %s", e.StatusCode, e.Message)
}

// CoolifyClient is a read-only, bearer-authenticated client for Coolify's REST API.
type CoolifyClient struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewCoolifyClient builds a CoolifyClient. baseURL is the Coolify
// instance root, e.g. "https://coolify.example.com"; "/api/v1" is
// appended per request, matching the openapi spec's own servers entry.
func NewCoolifyClient(baseURL, token string) *CoolifyClient {
	return &CoolifyClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CoolifyClient) do(ctx context.Context, path string, out any) error {
	return providerGetJSON(ctx, c.hc, c.baseURL+"/api/v1"+path, path, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
	}, func(status int, data []byte) error {
		return &coolifyAPIError{StatusCode: status, Message: extractCoolifyErrorMessage(data)}
	}, out)
}

func extractCoolifyErrorMessage(data []byte) string {
	var body coolifyErrorBody
	if err := json.Unmarshal(data, &body); err == nil && body.Message != "" {
		return body.Message
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// ListApplications calls GET /api/v1/applications.
func (c *CoolifyClient) ListApplications(ctx context.Context) ([]coolifyApplication, error) {
	var out []coolifyApplication
	err := c.do(ctx, "/applications", &out)
	return out, err
}

// GetApplication calls GET /api/v1/applications/{uuid}.
func (c *CoolifyClient) GetApplication(ctx context.Context, uuid string) (coolifyApplication, error) {
	var out coolifyApplication
	err := c.do(ctx, "/applications/"+pathEscape(uuid), &out)
	return out, err
}

// ListEnvs calls GET /api/v1/applications/{uuid}/envs.
func (c *CoolifyClient) ListEnvs(ctx context.Context, uuid string) ([]coolifyEnvVar, error) {
	var out []coolifyEnvVar
	err := c.do(ctx, "/applications/"+pathEscape(uuid)+"/envs", &out)
	return out, err
}
