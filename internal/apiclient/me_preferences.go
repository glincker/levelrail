package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
)

const meAppearancePath = "/api/v1/me/preferences/appearance"

// MePreference is a saved personal preference document; Value is null when unset.
type MePreference struct {
	Value json.RawMessage `json:"value"`
}

// GetMeAppearance calls GET /api/v1/me/preferences/appearance.
func (c *Client) GetMeAppearance(ctx context.Context) (MePreference, error) {
	var out MePreference
	err := c.do(ctx, http.MethodGet, meAppearancePath, nil, &out)
	return out, err
}

// SetMeAppearance calls PUT /api/v1/me/preferences/appearance with a JSON object.
func (c *Client) SetMeAppearance(ctx context.Context, doc json.RawMessage) (MePreference, error) {
	var out MePreference
	err := c.do(ctx, http.MethodPut, meAppearancePath, doc, &out)
	return out, err
}

// ResetMeAppearance calls DELETE /api/v1/me/preferences/appearance.
func (c *Client) ResetMeAppearance(ctx context.Context) (MePreference, error) {
	var out MePreference
	err := c.do(ctx, http.MethodDelete, meAppearancePath, nil, &out)
	return out, err
}
