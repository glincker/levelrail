package apiclient

import (
	"context"
	"net/http"
)

type ifMatchCtx struct{}

// WithIfMatch makes requests sent with the returned context carry If-Match for an optimistic write.
func WithIfMatch(ctx context.Context, revision string) context.Context {
	return context.WithValue(ctx, ifMatchCtx{}, revision)
}

func ifMatchFrom(ctx context.Context) string {
	v, _ := ctx.Value(ifMatchCtx{}).(string)
	return v
}

// RuntimeUlimit is one soft and hard limit.
type RuntimeUlimit struct {
	Soft int64 `json:"soft"`
	Hard int64 `json:"hard"`
}

// ServiceRuntimeOptions is the editable runtime block of one service.
type ServiceRuntimeOptions struct {
	Tmpfs       []string                 `json:"tmpfs,omitempty"`
	Ulimits     map[string]RuntimeUlimit `json:"ulimits,omitempty"`
	Init        *bool                    `json:"init,omitempty"`
	StopTimeout *int                     `json:"stop_timeout,omitempty"`
	ExtraHosts  []string                 `json:"extra_hosts,omitempty"`
	DNS         []string                 `json:"dns,omitempty"`
	Sysctls     map[string]string        `json:"sysctls,omitempty"`
	PidsLimit   int                      `json:"pids_limit,omitempty"`
}

// ServiceRuntimeBounds are the server's live limits.
type ServiceRuntimeBounds struct {
	TmpfsMax       int      `json:"tmpfs_max"`
	TmpfsBytesMax  int      `json:"tmpfs_bytes_max"`
	ExtraHostsMax  int      `json:"extra_hosts_max"`
	DNSMax         int      `json:"dns_max"`
	StopTimeoutMax int      `json:"stop_timeout_max_seconds"`
	PidsLimitMax   int      `json:"pids_limit_max"`
	UlimitCountMax int      `json:"ulimit_count_max"`
	UlimitBytesMax int      `json:"ulimit_bytes_max"`
	Ulimits        []string `json:"ulimits"`
	Sysctls        []string `json:"sysctls"`
}

// ServiceRuntime is a service's runtime options with the revision to send back as If-Match.
type ServiceRuntime struct {
	App      string                `json:"app"`
	Service  string                `json:"service"`
	Revision string                `json:"revision"`
	Runtime  ServiceRuntimeOptions `json:"runtime"`
	Editable []string              `json:"editable"`
	Bounds   ServiceRuntimeBounds  `json:"bounds"`
}

func serviceRuntimePath(app, service string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/services/" + PathEscape(service) + "/runtime"
}

// GetServiceRuntime calls GET /api/v1/apps/{name}/services/{service}/runtime.
func (c *Client) GetServiceRuntime(ctx context.Context, app, service string) (ServiceRuntime, error) {
	var out ServiceRuntime
	err := c.do(ctx, http.MethodGet, serviceRuntimePath(app, service), nil, &out)
	return out, err
}

// SetServiceRuntime calls PUT .../runtime. The body replaces every editable key; revision comes from a prior get.
func (c *Client) SetServiceRuntime(ctx context.Context, app, service, revision string, opts ServiceRuntimeOptions) (ServiceRuntime, error) {
	var out ServiceRuntime
	err := c.do(WithIfMatch(ctx, revision), http.MethodPut, serviceRuntimePath(app, service), opts, &out)
	return out, err
}
