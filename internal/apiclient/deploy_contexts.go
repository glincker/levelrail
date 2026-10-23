package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppContextConfig mirrors internal/api's contextConfigResource: one
// deploy context's env and build overrides.
type AppContextConfig struct {
	Context      string            `json:"context"`
	Env          map[string]string `json:"env"`
	BuildCommand string            `json:"build_command"`
	PublishDir   string            `json:"publish_dir"`
	BuildArgs    map[string]string `json:"build_args"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
}

// EffectiveEnvEntry is one resolved env var and where its value came from.
type EffectiveEnvEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
	Source string `json:"source"`
}

// EffectiveSetting is one resolved build setting and its source.
type EffectiveSetting struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// EffectiveConfig mirrors internal/deploycontext.Effective.
type EffectiveConfig struct {
	Context      string                      `json:"context"`
	Env          []EffectiveEnvEntry         `json:"env"`
	BuildCommand EffectiveSetting            `json:"build_command"`
	PublishDir   EffectiveSetting            `json:"publish_dir"`
	BuildArgs    map[string]EffectiveSetting `json:"build_args,omitempty"`
}

// ListAppContexts calls GET /api/v1/apps/{name}/contexts.
func (c *Client) ListAppContexts(ctx context.Context, name string) ([]AppContextConfig, error) {
	var out []AppContextConfig
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/contexts", nil, &out)
	return out, err
}

// SetAppContext calls PUT /api/v1/apps/{name}/contexts: it replaces one context's overrides.
func (c *Client) SetAppContext(ctx context.Context, name string, cfg AppContextConfig) (AppContextConfig, error) {
	var out AppContextConfig
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/contexts", cfg, &out)
	return out, err
}

// ClearAppContext calls DELETE /api/v1/apps/{name}/contexts?context=.
func (c *Client) ClearAppContext(ctx context.Context, name, contextName string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/contexts?context="+url.QueryEscape(contextName), nil, nil)
}

// GetEffectiveConfig calls GET /api/v1/apps/{name}/effective-config.
func (c *Client) GetEffectiveConfig(ctx context.Context, name, contextName, branch string) (EffectiveConfig, error) {
	q := url.Values{}
	if contextName != "" {
		q.Set("context", contextName)
	}
	if branch != "" {
		q.Set("branch", branch)
	}
	var out EffectiveConfig
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/effective-config?"+q.Encode(), nil, &out)
	return out, err
}
