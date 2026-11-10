package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DatabaseParam is one allowlisted engine parameter with its default,
// running and staged value.
type DatabaseParam struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Default     string   `json:"default"`
	Min         float64  `json:"min,omitempty"`
	Max         float64  `json:"max,omitempty"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description"`
	Current     string   `json:"current"`
	Pending     *string  `json:"pending,omitempty"`
}

// DatabaseParams is GET /api/v1/databases/{name}/params.
type DatabaseParams struct {
	Database        string            `json:"database"`
	Engine          string            `json:"engine"`
	Supported       bool              `json:"supported"`
	Params          []DatabaseParam   `json:"params"`
	HasPending      bool              `json:"has_pending"`
	RestartRequired bool              `json:"restart_required"`
	StagedBy        string            `json:"staged_by,omitempty"`
	StagedAt        string            `json:"staged_at,omitempty"`
	Previous        map[string]string `json:"previous"`
	MaxConnections  int               `json:"max_connections_ceiling"`
}

// DatabaseParamsChange is the body of PUT /api/v1/databases/{name}/params.
type DatabaseParamsChange struct {
	Set      map[string]string `json:"set,omitempty"`
	Unset    []string          `json:"unset,omitempty"`
	Rollback bool              `json:"rollback,omitempty"`
}

func databaseParamsPath(name string) string {
	return "/api/v1/databases/" + url.PathEscape(name) + "/params"
}

// GetDatabaseParams reads the parameter editor state.
func (c *Client) GetDatabaseParams(ctx context.Context, name string) (DatabaseParams, error) {
	var out DatabaseParams
	err := c.do(ctx, http.MethodGet, databaseParamsPath(name), nil, &out)
	return out, err
}

// StageDatabaseParams stages a change; nothing restarts.
func (c *Client) StageDatabaseParams(ctx context.Context, name string, change DatabaseParamsChange) (DatabaseParams, error) {
	var out DatabaseParams
	err := c.do(ctx, http.MethodPut, databaseParamsPath(name), change, &out)
	return out, err
}

// DiscardDatabaseParams drops the staged change.
func (c *Client) DiscardDatabaseParams(ctx context.Context, name string) (DatabaseParams, error) {
	var out DatabaseParams
	err := c.do(ctx, http.MethodDelete, databaseParamsPath(name)+"/pending", nil, &out)
	return out, err
}

// ApplyDatabaseParams promotes the staged change, which restarts the database.
func (c *Client) ApplyDatabaseParams(ctx context.Context, name string) (DatabaseParams, error) {
	var out DatabaseParams
	err := c.do(ctx, http.MethodPost, databaseParamsPath(name)+"/apply", map[string]bool{"confirm": true}, &out)
	return out, err
}
