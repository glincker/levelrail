package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// LoadBalancerSummary is one row of GET /api/v1/loadbalancers.
type LoadBalancerSummary struct {
	App               string     `json:"app"`
	Service           string     `json:"service"`
	Algorithm         string     `json:"algorithm"`
	State             string     `json:"state"`
	UpstreamsTotal    int        `json:"upstreams_total"`
	UpstreamsHealthy  int        `json:"upstreams_healthy"`
	Reason            string     `json:"reason,omitempty"`
	LastCheck         *time.Time `json:"last_check,omitempty"`
	ConfigUpdatedAt   string     `json:"config_updated_at"`
	ActiveHealthCheck bool       `json:"active_health_check"`
}

// LoadBalancerList is GET /api/v1/loadbalancers' body.
type LoadBalancerList struct {
	Items  []LoadBalancerSummary `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// LoadBalancerListParams filters and pages ListLoadBalancers; zero values are omitted.
type LoadBalancerListParams struct {
	State  string
	Search string
	Limit  int
	Offset int
}

// ListLoadBalancers calls GET /api/v1/loadbalancers.
func (c *Client) ListLoadBalancers(ctx context.Context, p LoadBalancerListParams) (LoadBalancerList, error) {
	q := url.Values{}
	if p.State != "" {
		q.Set("state", p.State)
	}
	if p.Search != "" {
		q.Set("q", p.Search)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	path := "/api/v1/loadbalancers"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out LoadBalancerList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
