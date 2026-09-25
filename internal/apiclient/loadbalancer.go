package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

// LoadBalancerConfig is a service's load balancer settings.
type LoadBalancerConfig = loadbalancer.Config

// LoadBalancerActiveHealth is the active health check block.
type LoadBalancerActiveHealth = loadbalancer.ActiveHealth

// LoadBalancerPassiveHealth is the passive health check block.
type LoadBalancerPassiveHealth = loadbalancer.PassiveHealth

// LoadBalancerRetries is the retry block.
type LoadBalancerRetries = loadbalancer.Retries

// LoadBalancerRateLimit is the rate limit block.
type LoadBalancerRateLimit = loadbalancer.RateLimit

// LoadBalancerUpstreamTLS is the upstream TLS block.
type LoadBalancerUpstreamTLS = loadbalancer.UpstreamTLS

// LoadBalancerStatus is the live view of a service's load balancer.
type LoadBalancerStatus = loadbalancer.Status

// LoadBalancerResource is GET/PUT /api/v1/apps/{name}/loadbalancer's body.
type LoadBalancerResource struct {
	AppName    string              `json:"app_name"`
	Configured bool                `json:"configured"`
	Config     *LoadBalancerConfig `json:"config,omitempty"`
	Algorithms []string            `json:"algorithms"`
	Formats    []string            `json:"export_formats"`
}

// LoadBalancerArtifact is one generated infrastructure-as-code file.
type LoadBalancerArtifact struct {
	Format      string   `json:"format"`
	Filename    string   `json:"filename"`
	ContentType string   `json:"content_type"`
	Body        string   `json:"body"`
	Warnings    []string `json:"warnings,omitempty"`
}

// GetLoadBalancer calls GET /api/v1/apps/{name}/loadbalancer.
func (c *Client) GetLoadBalancer(ctx context.Context, name string) (LoadBalancerResource, error) {
	var out LoadBalancerResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/loadbalancer", nil, &out)
	return out, err
}

// SetLoadBalancer calls PUT /api/v1/apps/{name}/loadbalancer.
func (c *Client) SetLoadBalancer(ctx context.Context, name string, cfg LoadBalancerConfig) (LoadBalancerResource, error) {
	var out LoadBalancerResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/loadbalancer", cfg, &out)
	return out, err
}

// DeleteLoadBalancer calls DELETE /api/v1/apps/{name}/loadbalancer.
func (c *Client) DeleteLoadBalancer(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/loadbalancer", nil, nil)
}

// GetLoadBalancerStatus calls GET /api/v1/apps/{name}/loadbalancer/status.
func (c *Client) GetLoadBalancerStatus(ctx context.Context, name string) (LoadBalancerStatus, error) {
	var out LoadBalancerStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/loadbalancer/status", nil, &out)
	return out, err
}

// ExportLoadBalancer calls GET /api/v1/apps/{name}/loadbalancer/export.
func (c *Client) ExportLoadBalancer(ctx context.Context, name, format string) (LoadBalancerArtifact, error) {
	var out LoadBalancerArtifact
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/loadbalancer/export?format="+url.QueryEscape(format), nil, &out)
	return out, err
}
