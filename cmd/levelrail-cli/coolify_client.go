package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// coolifyApplication mirrors the subset of Coolify's own Application
// model (docs-local/competitor-clones/coolify/openapi.json,
// #/components/schemas/Application) that field mapping actually reads.
// Deliberately partial: this client depends on Coolify's documented wire
// contract, not a full reimplementation of a model this project doesn't
// own.
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

// coolifyEnvVar mirrors Coolify's EnvironmentVariable model. Value is
// what GET .../envs returns without elevated ability (Coolify's API
// redacts it in that case); RealValue is only ever populated with a
// genuine plaintext value when the calling token carries the
// read:sensitive ability. Neither field is trusted by mapEnv unless the
// caller explicitly asked for real values (--include-secret-values), see
// coolify_map.go's mapEnv.
type coolifyEnvVar struct {
	UUID      string `json:"uuid"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	RealValue string `json:"real_value"`
}

type coolifyErrorBody struct {
	Message string `json:"message"`
}

// coolifyAPIError is returned by CoolifyClient methods for a non-2xx
// response: a real reply from Coolify, as opposed to a network-level
// failure, the same distinction apiError (client.go) draws for the
// Levelrail control plane's own API.
type coolifyAPIError struct {
	StatusCode int
	Message    string
}

func (e *coolifyAPIError) Error() string {
	return fmt.Sprintf("coolify api returned %d: %s", e.StatusCode, e.Message)
}

// CoolifyClient is a minimal, read-only HTTP client for Coolify's REST
// API, bearer-token authenticated per its own OpenAPI spec
// (docs-local/competitor-clones/coolify/openapi.json). This migration
// tool never writes back to the source Coolify instance.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil) //nolint:gosec // c.baseURL is the operator-supplied --url target this command exists to call, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &coolifyAPIError{StatusCode: resp.StatusCode, Message: extractCoolifyErrorMessage(data)}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
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
