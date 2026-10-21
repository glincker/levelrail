package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// FunctionSettings is the GET and PUT /api/v1/functions/{name}/settings body.
type FunctionSettings struct {
	Name                    string `json:"name"`
	Port                    int    `json:"port"`
	IdleMinutes             int    `json:"idle_minutes"`
	MemoryBytes             int64  `json:"memory_bytes"`
	NanoCPUs                int64  `json:"nano_cpus"`
	RequestTimeoutSeconds   int    `json:"request_timeout_seconds"`
	MaxConcurrency          int    `json:"max_concurrency"`
	EffectiveTimeoutSeconds int    `json:"effective_timeout_seconds"`
	EffectiveConcurrency    int    `json:"effective_concurrency"`
}

// FunctionSettingsUpdate sets the limits it carries; a nil field is left as it is and 0 restores the server default.
type FunctionSettingsUpdate struct {
	RequestTimeoutSeconds *int `json:"request_timeout_seconds,omitempty"`
	MaxConcurrency        *int `json:"max_concurrency,omitempty"`
}

// FunctionVersion is one deploy of a function.
type FunctionVersion struct {
	DeployID     string    `json:"deploy_id"`
	Image        string    `json:"image"`
	CommitSHA    string    `json:"commit_sha,omitempty"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	Live         bool      `json:"live"`
	Rollbackable bool      `json:"rollbackable"`
}

func functionPath(name, leaf string) string {
	return "/api/v1/functions/" + url.PathEscape(name) + "/" + leaf
}

// GetFunctionSettings calls GET /api/v1/functions/{name}/settings.
func (c *Client) GetFunctionSettings(ctx context.Context, name string) (FunctionSettings, error) {
	var out FunctionSettings
	err := c.do(ctx, http.MethodGet, functionPath(name, "settings"), nil, &out)
	return out, err
}

// SetFunctionSettings calls PUT /api/v1/functions/{name}/settings.
func (c *Client) SetFunctionSettings(ctx context.Context, name string, u FunctionSettingsUpdate) (FunctionSettings, error) {
	var out FunctionSettings
	err := c.do(ctx, http.MethodPut, functionPath(name, "settings"), u, &out)
	return out, err
}

// ListFunctionVersions calls GET /api/v1/functions/{name}/versions, newest first.
func (c *Client) ListFunctionVersions(ctx context.Context, name string) ([]FunctionVersion, error) {
	var out struct {
		Versions []FunctionVersion `json:"versions"`
	}
	err := c.do(ctx, http.MethodGet, functionPath(name, "versions"), nil, &out)
	return out.Versions, err
}

// RollbackFunction calls POST /api/v1/functions/{name}/rollback. An empty to picks the newest earlier version; pin also locks publishing.
func (c *Client) RollbackFunction(ctx context.Context, name, to string, pin bool) error {
	q := url.Values{}
	if to != "" {
		q.Set("to", to)
	}
	if pin {
		q.Set("pin", "true")
	}
	p := functionPath(name, "rollback")
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	return c.do(ctx, http.MethodPost, p, map[string]bool{"confirm": true}, nil)
}
