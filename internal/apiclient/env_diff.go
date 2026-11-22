package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// EnvDiff is the keys-only result of GET /api/v1/apps/{name}/env-diff.
type EnvDiff struct {
	App           string   `json:"app"`
	Against       string   `json:"against"`
	Added         []string `json:"added"`
	Removed       []string `json:"removed"`
	Changed       []string `json:"changed"`
	Same          int      `json:"same"`
	SecretUnknown int      `json:"secret_unchecked"`
}

// GetEnvDiff compares an app's env keys against an app, environment:<id> or preview.
func (c *Client) GetEnvDiff(ctx context.Context, app, against string) (EnvDiff, error) {
	var out EnvDiff
	q := url.Values{"against": {against}}
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/env-diff?"+q.Encode(), nil, &out)
	return out, err
}
