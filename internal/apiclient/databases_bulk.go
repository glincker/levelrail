package apiclient

import (
	"context"
	"net/http"
)

// BulkDatabasesRequest is POST /api/v1/databases/bulk's body.
type BulkDatabasesRequest struct {
	Action        string   `json:"action"`
	Names         []string `json:"names,omitempty"`
	Engine        string   `json:"engine,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	EnvironmentID string   `json:"environment_id,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
	ConfirmCount  int      `json:"confirm_count,omitempty"`
}

// BulkDatabaseResult is one database's outcome in a bulk response.
type BulkDatabaseResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	BackupID string `json:"backup_id,omitempty"`
}

// BulkDatabasesResponse is the 207 body of POST /api/v1/databases/bulk.
type BulkDatabasesResponse struct {
	Action  string               `json:"action"`
	DryRun  bool                 `json:"dry_run"`
	Results []BulkDatabaseResult `json:"results"`
	Counts  map[string]int       `json:"counts"`
}

// BulkDatabases calls POST /api/v1/databases/bulk.
func (c *Client) BulkDatabases(ctx context.Context, req BulkDatabasesRequest) (BulkDatabasesResponse, error) {
	var out BulkDatabasesResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/bulk", req, &out)
	return out, err
}
