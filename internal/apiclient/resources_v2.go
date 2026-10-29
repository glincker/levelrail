package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// V2Metadata is the uniform metadata block of the v2 resource view.
type V2Metadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	ManagedFields     []V2ManagedField  `json:"managedFields,omitempty"`
}

// V2ManagedField lists the spec fields one server-side apply manager owns.
type V2ManagedField struct {
	Manager string   `json:"manager"`
	Fields  []string `json:"fields"`
}

// V2Object mirrors one item of GET /api/v2/{apps,databases,nodes}.
type V2Object struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   V2Metadata     `json:"metadata"`
	Spec       map[string]any `json:"spec,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
}

// V2List mirrors the list envelope of the v2 view.
type V2List struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion"`
		Continue        string `json:"continue,omitempty"`
	} `json:"metadata"`
	Items []V2Object `json:"items"`
}

// ListV2 calls GET /api/v2/{path} with an optional label selector and page size.
func (c *Client) ListV2(ctx context.Context, path, labelSelector string, limit int) (V2List, error) {
	q := url.Values{}
	if labelSelector != "" {
		q.Set("labelSelector", labelSelector)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	p := "/api/v2/" + path
	if enc := q.Encode(); enc != "" {
		p += "?" + enc
	}
	var out V2List
	err := c.do(ctx, http.MethodGet, p, nil, &out)
	return out, err
}

// GetV2 calls GET /api/v2/{path}/{name}.
func (c *Client) GetV2(ctx context.Context, path, name string) (V2Object, error) {
	var out V2Object
	err := c.do(ctx, http.MethodGet, "/api/v2/"+path+"/"+PathEscape(name), nil, &out)
	return out, err
}

// V2ApplyOptions are the query parameters of a server-side apply.
type V2ApplyOptions struct {
	FieldManager    string
	Force           bool
	DryRun          bool
	ResourceVersion string
}

// V2ApplyResult is the object after a server-side apply, or the merge a dry run would write.
type V2ApplyResult struct {
	V2Object
	DryRun bool `json:"dryRun,omitempty"`
}

// ApplyV2 calls PATCH /api/v2/{path}/{name} with a partial manifest, merging only
// the fields opts.FieldManager sends.
func (c *Client) ApplyV2(ctx context.Context, path, name string, manifest map[string]any, opts V2ApplyOptions) (V2ApplyResult, error) {
	q := url.Values{}
	q.Set("fieldManager", opts.FieldManager)
	if opts.Force {
		q.Set("force", "true")
	}
	if opts.DryRun {
		q.Set("dryRun", "All")
	}
	if opts.ResourceVersion != "" {
		q.Set("resourceVersion", opts.ResourceVersion)
	}
	var out V2ApplyResult
	err := c.do(ctx, http.MethodPatch, "/api/v2/"+path+"/"+PathEscape(name)+"?"+q.Encode(), manifest, &out)
	return out, err
}
