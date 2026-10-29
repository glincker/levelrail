package apiclient

import (
	"context"
	"net/http"
)

// LabelSet is the body of the label endpoints.
type LabelSet struct {
	Labels map[string]string `json:"labels"`
}

// GetResourceLabels calls GET /api/v1/{path}/{name}/labels for apps, databases, stacks or nodes.
func (c *Client) GetResourceLabels(ctx context.Context, path, name string) (LabelSet, error) {
	var out LabelSet
	err := c.do(ctx, http.MethodGet, "/api/v1/"+path+"/"+PathEscape(name)+"/labels", nil, &out)
	return out, err
}

// PatchResourceLabels sets and removes keys in one call; nodes only have a replace route, so it is merged here.
func (c *Client) PatchResourceLabels(ctx context.Context, path, name string, set map[string]string, remove []string) (LabelSet, error) {
	var out LabelSet
	p := "/api/v1/" + path + "/" + PathEscape(name) + "/labels"
	if path != "nodes" {
		err := c.do(ctx, http.MethodPatch, p, map[string]any{"set": set, "remove": remove}, &out)
		return out, err
	}
	cur, err := c.GetResourceLabels(ctx, path, name)
	if err != nil {
		return out, err
	}
	next := map[string]string{}
	for k, v := range cur.Labels {
		next[k] = v
	}
	for _, k := range remove {
		delete(next, k)
	}
	for k, v := range set {
		next[k] = v
	}
	err = c.do(ctx, http.MethodPut, p, LabelSet{Labels: next}, &out)
	return out, err
}
