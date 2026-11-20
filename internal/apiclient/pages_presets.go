package apiclient

import (
	"context"
	"net/http"
)

// PagesPreset is one named performance preset.
type PagesPreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Compression []string `json:"compression"`
}

// PagesPresetChoice is the preset set on one site; empty means none.
type PagesPresetChoice struct {
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

// ListPagesPresets calls GET /api/v1/pages/presets.
func (c *Client) ListPagesPresets(ctx context.Context) ([]PagesPreset, error) {
	var out struct {
		Presets []PagesPreset `json:"presets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/pages/presets", nil, &out)
	return out.Presets, err
}

// GetStaticSitePreset calls GET /api/v1/static-sites/{name}/preset.
func (c *Client) GetStaticSitePreset(ctx context.Context, name string) (PagesPresetChoice, error) {
	var out PagesPresetChoice
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/preset", nil, &out)
	return out, err
}

// SetStaticSitePreset calls PUT /api/v1/static-sites/{name}/preset; "" clears it.
func (c *Client) SetStaticSitePreset(ctx context.Context, name, preset string) (PagesPresetChoice, error) {
	var out PagesPresetChoice
	err := c.do(ctx, http.MethodPut, "/api/v1/static-sites/"+PathEscape(name)+"/preset", map[string]string{"preset": preset}, &out)
	return out, err
}
