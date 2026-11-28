package apiclient

import (
	"context"
	"net/http"
)

// EnvMergeRequest is a batch of plain env changes applied in one server-side transaction.
type EnvMergeRequest struct {
	Set   map[string]string `json:"set,omitempty"`
	Unset []string          `json:"unset,omitempty"`
}

// EnvMergeResult lists key names only, never a value.
type EnvMergeResult struct {
	App         string   `json:"app"`
	Changed     bool     `json:"changed"`
	EnvDirty    bool     `json:"env_dirty"`
	Set         []string `json:"set"`
	Removed     []string `json:"removed"`
	Missing     []string `json:"missing"`
	Keys        []string `json:"keys"`
	RebuildKeys []string `json:"rebuild_keys,omitempty"`
}

// MergeAppEnv calls PUT /api/v1/apps/{name}/env. Unlike UpdateApp it touches
// only the env column, so a concurrent full-spec write cannot lose these keys.
func (c *Client) MergeAppEnv(ctx context.Context, name string, req EnvMergeRequest) (EnvMergeResult, error) {
	var out EnvMergeResult
	if err := PrecheckEnv(req.Set, false); err != nil {
		return out, err
	}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/env", req, &out)
	return out, err
}
