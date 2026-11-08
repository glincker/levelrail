package apiclient

import (
	"context"
	"net/http"
)

// SizePreset mirrors one entry of GET /api/v1/size-presets.
type SizePreset struct {
	Name        string  `json:"name"`
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// SizePresets calls GET /api/v1/size-presets.
func (c *Client) SizePresets(ctx context.Context) ([]SizePreset, error) {
	var out struct {
		Presets []SizePreset `json:"presets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/size-presets", nil, &out)
	return out.Presets, err
}
