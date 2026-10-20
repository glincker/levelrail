package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ListNodesOptions are the GET /api/v1/nodes filters, mirroring
// DeploymentListOptions' Values() shape.
type ListNodesOptions struct {
	Query  string
	Limit  int
	Offset int
}

// Values renders the options as a query string, omitting unset filters.
func (o ListNodesOptions) Values() url.Values {
	v := url.Values{}
	if o.Query != "" {
		v.Set("q", o.Query)
	}
	if o.Limit > 0 {
		v.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Offset > 0 {
		v.Set("offset", strconv.Itoa(o.Offset))
	}
	return v
}

// ListNodesFiltered calls GET /api/v1/nodes with optional query, limit
// and offset, returning nodes plus the total match count before
// paging (the X-Total-Count header handleListNodes always sets).
// ListNodes stays the plain unfiltered call every existing caller
// already uses.
func (c *Client) ListNodesFiltered(ctx context.Context, opts ListNodesOptions) ([]NodeResource, int, error) {
	path := nodesCollectionPath()
	if q := opts.Values().Encode(); q != "" {
		path += "?" + q
	}
	var out []NodeResource
	headers, err := c.doHeaders(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return nil, 0, err
	}
	total, _ := strconv.Atoi(headers.Get("X-Total-Count"))
	return out, total, nil
}
