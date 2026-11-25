package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// EnvRevisionRestore is the names-only result of restoring an env revision.
type EnvRevisionRestore struct {
	App      string   `json:"app"`
	Restored int      `json:"restored"`
	Revision int      `json:"revision"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Changed  []string `json:"changed"`
}

// RestoreEnvRevision calls POST /api/v1/apps/{name}/env/revisions/{rev}/restore. confirm must equal the app name.
func (c *Client) RestoreEnvRevision(ctx context.Context, name string, rev int, confirm string) (EnvRevisionRestore, error) {
	var out EnvRevisionRestore
	body := struct {
		Confirm string `json:"confirm"`
	}{confirm}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/env/revisions/"+strconv.Itoa(rev)+"/restore", body, &out)
	return out, err
}
