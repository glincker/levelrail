package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ReadinessCheck is one server readiness check.
type ReadinessCheck struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

// ServerReadiness is GET /api/v1/system/readiness' response.
type ServerReadiness struct {
	Checks   []ReadinessCheck `json:"checks"`
	Mode     string           `json:"mode"`
	Proxy    string           `json:"proxy"`
	Holders  map[int]string   `json:"holders"`
	NextStep string           `json:"next_step"`
	Blocked  bool             `json:"blocked"`
	Summary  string           `json:"summary"`
}

// GetServerReadiness calls GET /api/v1/system/readiness, optionally also
// checking the DNS of domain.
func (c *Client) GetServerReadiness(ctx context.Context, domain string) (ServerReadiness, error) {
	var out ServerReadiness
	path := "/api/v1/system/readiness"
	if domain != "" {
		path += "?domain=" + url.QueryEscape(domain)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
