package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
)

// ListTokensQuery calls GET /api/v1/auth/tokens with filter, sort and page
// parameters already encoded, returning the page undecoded.
func (c *Client) ListTokensQuery(ctx context.Context, rawQuery string) (json.RawMessage, error) {
	return getJSON[json.RawMessage](ctx, c, "/api/v1/auth/tokens?"+rawQuery)
}

type tokenPruneRequest struct {
	OlderThanDays int  `json:"older_than_days"`
	DryRun        bool `json:"dry_run"`
}

// TokenPruneResult is the body of POST /api/v1/tokens/prune.
type TokenPruneResult struct {
	DryRun  bool `json:"dry_run"`
	Revoked int  `json:"revoked"`
	Expired int  `json:"expired"`
}

// PruneTokens calls POST /api/v1/tokens/prune (root ability). With dryRun it only counts.
func (c *Client) PruneTokens(ctx context.Context, olderThanDays int, dryRun bool) (TokenPruneResult, error) {
	var out TokenPruneResult
	err := c.do(ctx, http.MethodPost, "/api/v1/tokens/prune", tokenPruneRequest{OlderThanDays: olderThanDays, DryRun: dryRun}, &out)
	return out, err
}
