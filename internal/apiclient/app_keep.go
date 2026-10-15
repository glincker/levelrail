package apiclient

import (
	"context"
	"net/http"
	"time"
)

// KeepTrialResult mirrors internal/api's keepTrialResponse.
type KeepTrialResult struct {
	Name        string    `json:"name"`
	Kept        []string  `json:"kept"`
	KeptAt      time.Time `json:"kept_at"`
	AlreadyKept bool      `json:"already_kept"`
}

// KeepTrialApp calls POST /api/v1/apps/{name}/keep: a trial deployment
// becomes a permanent app.
func (c *Client) KeepTrialApp(ctx context.Context, name string) (KeepTrialResult, error) {
	var out KeepTrialResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/keep", nil, &out)
	return out, err
}
