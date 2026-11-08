package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// DatabaseColumnResource is one column in GET /databases/{name}/schema.
type DatabaseColumnResource struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	Default    string `json:"default,omitempty"`
	PrimaryKey bool   `json:"primary_key"`
}

// DatabaseIndexResource is one index in the schema listing.
type DatabaseIndexResource struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Unique     bool   `json:"unique"`
	Primary    bool   `json:"primary"`
}

// DatabaseTableResource is one table or view in the schema listing.
type DatabaseTableResource struct {
	Name        string                   `json:"name"`
	Kind        string                   `json:"kind"`
	RowEstimate int64                    `json:"row_estimate"`
	SizeBytes   int64                    `json:"size_bytes"`
	Columns     []DatabaseColumnResource `json:"columns"`
	Indexes     []DatabaseIndexResource  `json:"indexes"`
}

// DatabaseSchemaNodeResource groups tables under one schema.
type DatabaseSchemaNodeResource struct {
	Name   string                  `json:"name"`
	Tables []DatabaseTableResource `json:"tables"`
}

// DatabaseSchemaResource is GET /api/v1/databases/{name}/schema.
type DatabaseSchemaResource struct {
	Engine  string                       `json:"engine"`
	Schemas []DatabaseSchemaNodeResource `json:"schemas"`
}

// DatabaseQueryRequest is the body of the query, write, and explain routes.
type DatabaseQueryRequest struct {
	SQL     string `json:"sql"`
	Analyze bool   `json:"analyze,omitempty"`
	Confirm string `json:"confirm,omitempty"`
}

// DatabaseQueryResult is the response of the query routes. A nil cell is NULL.
type DatabaseQueryResult struct {
	Columns     []string    `json:"columns"`
	Rows        [][]*string `json:"rows"`
	RowCount    int         `json:"row_count"`
	Truncated   bool        `json:"truncated"`
	DurationMs  int64       `json:"duration_ms"`
	Mode        string      `json:"mode"`
	Kind        string      `json:"kind"`
	Fingerprint string      `json:"fingerprint"`
}

// GetDatabaseSchema calls GET /api/v1/databases/{name}/schema.
func (c *Client) GetDatabaseSchema(ctx context.Context, name string) (DatabaseSchemaResource, error) {
	var out DatabaseSchemaResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/schema", nil, &out)
	return out, err
}

// QueryDatabase calls POST /api/v1/databases/{name}/query, or /query/write
// when write is true (admin only, confirm must equal the database name).
func (c *Client) QueryDatabase(ctx context.Context, name string, req DatabaseQueryRequest, write bool) (DatabaseQueryResult, error) {
	path := "/api/v1/databases/" + PathEscape(name) + "/query"
	if write {
		path += "/write"
	}
	var out DatabaseQueryResult
	err := c.do(ctx, http.MethodPost, path, req, &out)
	return out, err
}

// ExplainDatabaseQuery calls POST /api/v1/databases/{name}/explain.
func (c *Client) ExplainDatabaseQuery(ctx context.Context, name string, req DatabaseQueryRequest) (DatabaseQueryResult, error) {
	var out DatabaseQueryResult
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/explain", req, &out)
	return out, err
}

// DatabaseEngineObject is one index or collection and its count.
type DatabaseEngineObject struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// DatabaseEngineStats is GET /api/v1/databases/{name}/engine-stats.
type DatabaseEngineStats struct {
	Engine  string                 `json:"engine"`
	Objects []DatabaseEngineObject `json:"objects"`
	Total   int64                  `json:"total"`
	Metrics map[string]int64       `json:"metrics,omitempty"`
}

// DatabaseQueryHistoryEntry is one row of GET /databases/{name}/query-history.
type DatabaseQueryHistoryEntry struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	SQL         string `json:"sql"`
	Fingerprint string `json:"fingerprint"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	RowCount    int    `json:"row_count"`
	CreatedAt   string `json:"created_at"`
}

// DatabaseSavedQuery is one row of GET /databases/{name}/saved-queries.
type DatabaseSavedQuery struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SQL       string `json:"sql"`
	CreatedAt string `json:"created_at"`
}

// GetDatabaseEngineStats calls GET /api/v1/databases/{name}/engine-stats.
func (c *Client) GetDatabaseEngineStats(ctx context.Context, name string) (DatabaseEngineStats, error) {
	var out DatabaseEngineStats
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/engine-stats", nil, &out)
	return out, err
}

// ListDatabaseQueryHistory calls GET /api/v1/databases/{name}/query-history.
func (c *Client) ListDatabaseQueryHistory(ctx context.Context, name string, limit int) ([]DatabaseQueryHistoryEntry, error) {
	path := "/api/v1/databases/" + PathEscape(name) + "/query-history"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out []DatabaseQueryHistoryEntry
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ClearDatabaseQueryHistory calls DELETE /api/v1/databases/{name}/query-history.
func (c *Client) ClearDatabaseQueryHistory(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/databases/"+PathEscape(name)+"/query-history", nil, nil)
}

// ListDatabaseSavedQueries calls GET /api/v1/databases/{name}/saved-queries.
func (c *Client) ListDatabaseSavedQueries(ctx context.Context, name string) ([]DatabaseSavedQuery, error) {
	var out []DatabaseSavedQuery
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/saved-queries", nil, &out)
	return out, err
}

// SaveDatabaseQuery calls POST /api/v1/databases/{name}/saved-queries.
func (c *Client) SaveDatabaseQuery(ctx context.Context, name, title, sql string) (DatabaseSavedQuery, error) {
	var out DatabaseSavedQuery
	body := map[string]string{"name": title, "sql": sql}
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/saved-queries", body, &out)
	return out, err
}

// DeleteDatabaseSavedQuery calls DELETE /api/v1/databases/{name}/saved-queries/{id}.
func (c *Client) DeleteDatabaseSavedQuery(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/databases/"+PathEscape(name)+"/saved-queries/"+PathEscape(id), nil, nil)
}
