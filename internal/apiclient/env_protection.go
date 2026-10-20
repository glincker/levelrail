package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// MaskedEnvValue is what a sensitive env value reads back as.
const MaskedEnvValue = "••••••••"

// StepUpProof is the account owner's password, plus the authenticator or
// recovery code when the account has two-factor authentication.
type StepUpProof struct {
	Password     string `json:"password,omitempty"`
	Code         string `json:"code,omitempty"`
	RecoveryCode string `json:"recovery_code,omitempty"`
}

// RevealResult is one revealed value.
type RevealResult struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	Type             string `json:"type"`
	HideAfterSeconds int    `json:"hide_after_seconds"`
}

type revealEnvRequest struct {
	Key    string       `json:"key,omitempty"`
	StepUp *StepUpProof `json:"step_up,omitempty"`
}

// RevealEnv calls POST /api/v1/apps/{name}/env/reveal.
func (c *Client) RevealEnv(ctx context.Context, app, key string, proof *StepUpProof) (RevealResult, error) {
	var out RevealResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/env/reveal", revealEnvRequest{Key: key, StepUp: proof}, &out)
	return out, err
}

// RevealSecret calls POST /api/v1/apps/{name}/secrets/{key}/reveal.
func (c *Client) RevealSecret(ctx context.Context, app, key string, proof *StepUpProof) (RevealResult, error) {
	var out RevealResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/secrets/"+PathEscape(key)+"/reveal", revealEnvRequest{StepUp: proof}, &out)
	return out, err
}

// ExportEnv calls GET /api/v1/apps/{name}/env/export: the file with
// sensitive values masked (values "masked") or every value dropped ("none").
func (c *Client) ExportEnv(ctx context.Context, app, format, values string) ([]byte, error) {
	q := url.Values{}
	q.Set("format", format)
	q.Set("values", values)
	return c.rawRequest(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/env/export?"+q.Encode(), nil)
}

type exportEnvRequest struct {
	Format string       `json:"format"`
	StepUp *StepUpProof `json:"step_up,omitempty"`
}

// ExportEnvRevealed calls POST /api/v1/apps/{name}/env/export: every value,
// secrets included, after a step-up.
func (c *Client) ExportEnvRevealed(ctx context.Context, app, format string, proof *StepUpProof) ([]byte, error) {
	return c.rawRequest(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/env/export", exportEnvRequest{Format: format, StepUp: proof})
}

// BoundEnvEntry is one variable injected from a database link or bucket binding.
type BoundEnvEntry struct {
	Key        string `json:"key"`
	Source     string `json:"source"`
	SourceName string `json:"source_name"`
	Field      string `json:"field"`
	Value      string `json:"value,omitempty"`
	Masked     bool   `json:"masked"`
}

// ListBoundEnv calls GET /api/v1/apps/{name}/env/bound.
func (c *Client) ListBoundEnv(ctx context.Context, app string) ([]BoundEnvEntry, error) {
	var out []BoundEnvEntry
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/env/bound", nil, &out)
	return out, err
}

// EnvSettings mirrors GET /api/v1/apps/{name}/env/settings.
type EnvSettings struct {
	RevealPolicy       string          `json:"reveal_policy"`
	PolicyValid        bool            `json:"policy_valid"`
	TokenReveal        bool            `json:"token_reveal"`
	SecretsWriteOnly   bool            `json:"secrets_write_only"`
	SensitiveOverrides map[string]bool `json:"sensitive_overrides"`
	GrantTTLSeconds    int             `json:"grant_ttl_seconds"`
	HideAfterSeconds   int             `json:"hide_after_seconds"`
	SecretsConfigured  bool            `json:"secrets_configured"`
}

// EnvSettingsUpdate is PUT /api/v1/apps/{name}/env/settings's body. A nil
// override value clears that key's override.
type EnvSettingsUpdate struct {
	SensitiveOverrides map[string]*bool `json:"sensitive_overrides,omitempty"`
	SecretsWriteOnly   *bool            `json:"secrets_write_only,omitempty"`
	StepUp             *StepUpProof     `json:"step_up,omitempty"`
}

// GetEnvSettings calls GET /api/v1/apps/{name}/env/settings.
func (c *Client) GetEnvSettings(ctx context.Context, app string) (EnvSettings, error) {
	var out EnvSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/env/settings", nil, &out)
	return out, err
}

// UpdateEnvSettings calls PUT /api/v1/apps/{name}/env/settings.
func (c *Client) UpdateEnvSettings(ctx context.Context, app string, in EnvSettingsUpdate) (EnvSettings, error) {
	var out EnvSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(app)+"/env/settings", in, &out)
	return out, err
}

// ConvertEnvToSecret calls POST /api/v1/apps/{name}/env/{key}/convert-to-secret.
func (c *Client) ConvertEnvToSecret(ctx context.Context, app, key string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/env/"+PathEscape(key)+"/convert-to-secret", nil, nil)
}

// rawRequest sends an optional JSON body and returns the raw response body.
func (c *Client) rawRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody) //nolint:gosec // operator-supplied API target, same as do()
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set(headerContentType, contentTypeJSON)
	}
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		TraceRequest(method, c.baseURL+path, "", time.Since(start), err)
		return nil, fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(method, c.baseURL+path, resp.Status, time.Since(start), nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeResponse(resp, nil)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}
