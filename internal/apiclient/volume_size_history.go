package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// VolumeSizePoint is one used-bytes sample of a volume.
type VolumeSizePoint struct {
	At        time.Time `json:"at"`
	UsedBytes float64   `json:"used_bytes"`
}

// VolumeSizeHistory calls GET /api/v1/apps/{name}/volumes/{volume}/size-history; empty from or to use the server default.
func (c *Client) VolumeSizeHistory(ctx context.Context, app, volume, from, to string) ([]VolumeSizePoint, error) {
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if to != "" {
		q.Set("to", to)
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/volumes/" + PathEscape(volume) + "/size-history"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []VolumeSizePoint
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
