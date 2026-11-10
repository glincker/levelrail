package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// SavedView is a named list filter, private to its owner or shared with the owner's team.
type SavedView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	List      string            `json:"list"`
	Filter    map[string]string `json:"filter"`
	Shared    string            `json:"shared"`
	Mine      bool              `json:"mine"`
	CanDelete bool              `json:"can_delete"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// SavedViewInput is the POST body; Shared is private or team.
type SavedViewInput struct {
	Name   string            `json:"name"`
	List   string            `json:"list"`
	Filter map[string]string `json:"filter"`
	Shared string            `json:"shared,omitempty"`
}

// ListSavedViews calls GET /api/v1/saved-views, optionally for one list.
func (c *Client) ListSavedViews(ctx context.Context, list string) ([]SavedView, error) {
	path := "/api/v1/saved-views"
	if list != "" {
		path += "?" + url.Values{"list": {list}}.Encode()
	}
	var out struct {
		Views []SavedView `json:"views"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Views, err
}

// SaveView calls POST /api/v1/saved-views.
func (c *Client) SaveView(ctx context.Context, in SavedViewInput) (SavedView, error) {
	var out SavedView
	err := c.do(ctx, http.MethodPost, "/api/v1/saved-views", in, &out)
	return out, err
}

// DeleteSavedView calls DELETE /api/v1/saved-views/{id}.
func (c *Client) DeleteSavedView(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/saved-views/"+url.PathEscape(id), nil, nil)
}
