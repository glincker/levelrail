package apiclient

import (
	"context"
	"net/http"
)

// ComposeDBRestoreRequest is the body of the compose database restore route.
type ComposeDBRestoreRequest struct {
	Confirm string `json:"confirm,omitempty"`
	Preview bool   `json:"preview,omitempty"`
}

// ComposeDBRestoreResource describes a restore preview or an accepted restore.
type ComposeDBRestoreResource struct {
	RunID         string `json:"run_id"`
	ObjectKey     string `json:"object_key"`
	SizeBytes     int64  `json:"size_bytes"`
	Engine        string `json:"engine"`
	App           string `json:"app"`
	Service       string `json:"service"`
	ConfirmPhrase string `json:"confirm_phrase"`
	Preview       bool   `json:"preview"`
	SafetyRunID   string `json:"safety_run_id,omitempty"`
}

// RestoreComposeDBBackup calls POST /api/v1/apps/{name}/compose-db-services/{service}/backups/{run}/restore.
func (c *Client) RestoreComposeDBBackup(ctx context.Context, app, service, run string, req ComposeDBRestoreRequest) (ComposeDBRestoreResource, error) {
	var out ComposeDBRestoreResource
	path := "/api/v1/apps/" + PathEscape(app) + "/compose-db-services/" + PathEscape(service) + "/backups/" + PathEscape(run) + "/restore"
	err := c.do(ctx, http.MethodPost, path, req, &out)
	return out, err
}
