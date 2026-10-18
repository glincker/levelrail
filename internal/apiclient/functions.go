package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// FunctionResource mirrors one entry of GET /api/v1/functions.
type FunctionResource struct {
	Name         string  `json:"name"`
	Image        string  `json:"image"`
	Port         int     `json:"port"`
	IdleMinutes  int     `json:"idle_minutes"`
	Sleeping     bool    `json:"sleeping"`
	URL          string  `json:"url,omitempty"`
	Invocations  float64 `json:"invocations"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P95Ms        float64 `json:"p95_ms"`
}

// FunctionsList is the GET /api/v1/functions response.
type FunctionsList struct {
	WindowSeconds float64            `json:"window_seconds"`
	Functions     []FunctionResource `json:"functions"`
}

// ListFunctions returns every function with call count and p95 duration over window (1h, 24h or 7d; empty means 24h).
func (c *Client) ListFunctions(ctx context.Context, window string) (FunctionsList, error) {
	path := "/api/v1/functions"
	if window != "" {
		path += "?window=" + url.QueryEscape(window)
	}
	var out FunctionsList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
