package apiclient

import (
	"context"
	"net/http"
)

// PagesNotifyChannel is a channel attached to a site; destinations are never returned.
type PagesNotifyChannel struct {
	TargetID  string `json:"target_id"`
	ChannelID string `json:"channel_id"`
	Enabled   bool   `json:"enabled"`
}

// PagesNotify is a site's deploy notification state.
type PagesNotify struct {
	Site     string               `json:"site"`
	Enabled  bool                 `json:"enabled"`
	Events   []string             `json:"events"`
	Explicit bool                 `json:"explicit"`
	Channels []PagesNotifyChannel `json:"channels"`
}

// PagesNotifyUpdate is the PUT body; nil Enabled and nil Events keep the stored values.
type PagesNotifyUpdate struct {
	Enabled   *bool    `json:"enabled,omitempty"`
	Events    []string `json:"events,omitempty"`
	ChannelID string   `json:"channel_id,omitempty"`
}

// GetPagesNotify reads a site's deploy notification settings.
func (c *Client) GetPagesNotify(ctx context.Context, site string) (PagesNotify, error) {
	var out PagesNotify
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(site)+"/notify", nil, &out)
	return out, err
}

// PutPagesNotify updates the switch, the event filter or attaches a channel.
func (c *Client) PutPagesNotify(ctx context.Context, site string, u PagesNotifyUpdate) (PagesNotify, error) {
	var out PagesNotify
	err := c.do(ctx, http.MethodPut, "/api/v1/static-sites/"+PathEscape(site)+"/notify", u, &out)
	return out, err
}

// ResetPagesNotify drops the filter so the site notifies on every outcome again.
func (c *Client) ResetPagesNotify(ctx context.Context, site string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/static-sites/"+PathEscape(site)+"/notify", nil, nil)
}
