package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ResourceMeta is metadata of one v2 resource.
type ResourceMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

// Resource is one object of the v2 uniform resource view; spec and status stay raw per kind.
type Resource struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ResourceMeta    `json:"metadata"`
	Spec       json.RawMessage `json:"spec,omitempty"`
	Status     json.RawMessage `json:"status,omitempty"`
}

// ResourceList is a v2 list response.
type ResourceList struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion"`
		Continue        string `json:"continue,omitempty"`
	} `json:"metadata"`
	Items []Resource `json:"items"`
}

// ResourceEvent is one v2 watch event: ADDED, MODIFIED, DELETED, BOOKMARK or ERROR.
type ResourceEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

// ResourceListOptions narrows a v2 list or watch.
type ResourceListOptions struct {
	LabelSelector   string
	Limit           int
	Continue        string
	ResourceVersion string
}

func (o ResourceListOptions) query(watch bool) string {
	q := url.Values{}
	if o.LabelSelector != "" {
		q.Set("labelSelector", o.LabelSelector)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Continue != "" {
		q.Set("continue", o.Continue)
	}
	if watch {
		q.Set("watch", "true")
		if o.ResourceVersion != "" {
			q.Set("resourceVersion", o.ResourceVersion)
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// ListResources lists kind (apps, nodes, databases) through the v2 view.
func (c *Client) ListResources(ctx context.Context, kind string, opts ResourceListOptions) (*ResourceList, error) {
	var out ResourceList
	if err := c.do(ctx, http.MethodGet, "/api/v2/"+url.PathEscape(kind)+opts.query(false), nil, &out); err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}
	return &out, nil
}

// GetResource reads one object of kind by name through the v2 view.
func (c *Client) GetResource(ctx context.Context, kind, name string) (*Resource, error) {
	var out Resource
	if err := c.do(ctx, http.MethodGet, "/api/v2/"+url.PathEscape(kind)+"/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, fmt.Errorf("get %s %s: %w", kind, name, err)
	}
	return &out, nil
}

// WatchResources streams changes of kind after opts.ResourceVersion until ctx
// ends or onEvent errors. IsResourceExpired reports when the caller must relist.
func (c *Client) WatchResources(ctx context.Context, kind string, opts ResourceListOptions, onEvent func(ResourceEvent) error) error {
	return streamSSE(ctx, c, http.MethodGet, "/api/v2/"+url.PathEscape(kind)+opts.query(true), nil, onEvent)
}

// IsResourceExpired reports whether err is the 410 a watch returns for a compacted resourceVersion.
func IsResourceExpired(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusGone
}
