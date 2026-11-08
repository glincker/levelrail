package apiclient

import (
	"context"
	"net/http"
)

// SizePreset is one named CPU and memory size.
type SizePreset struct {
	Name        string  `json:"name"`
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// ListSizePresets calls GET /api/v1/size-presets.
func (c *Client) ListSizePresets(ctx context.Context) ([]SizePreset, error) {
	var out struct {
		Presets []SizePreset `json:"presets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/size-presets", nil, &out)
	return out.Presets, err
}
