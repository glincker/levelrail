package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// MongoCollectionsResult is the response of GET /databases/{name}/mongo/collections.
type MongoCollectionsResult struct {
	Databases   []json.RawMessage `json:"databases"`
	Collections []json.RawMessage `json:"collections"`
	Truncated   bool              `json:"truncated"`
	Fingerprint string            `json:"fingerprint"`
}

// MongoFindRequest is the body of POST /databases/{name}/mongo/find. The
// server's allow-list guard is the only check; the client sends values as given.
type MongoFindRequest struct {
	Database   string          `json:"database"`
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter,omitempty"`
	Sort       json.RawMessage `json:"sort,omitempty"`
	Projection json.RawMessage `json:"projection,omitempty"`
	Limit      int             `json:"limit,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
}

// MongoFindResult is the response of the find route.
type MongoFindResult struct {
	Documents   []json.RawMessage `json:"documents"`
	Truncated   bool              `json:"truncated"`
	NextAfter   json.RawMessage   `json:"next_after,omitempty"`
	Fingerprint string            `json:"fingerprint"`
	DurationMs  int64             `json:"duration_ms"`
}

// ListMongoCollections calls GET /api/v1/databases/{name}/mongo/collections.
func (c *Client) ListMongoCollections(ctx context.Context, name, database string) (MongoCollectionsResult, error) {
	path := "/api/v1/databases/" + PathEscape(name) + "/mongo/collections"
	if database != "" {
		path += "?database=" + url.QueryEscape(database)
	}
	return getJSON[MongoCollectionsResult](ctx, c, path)
}

// MongoFind calls POST /api/v1/databases/{name}/mongo/find.
func (c *Client) MongoFind(ctx context.Context, name string, req MongoFindRequest) (MongoFindResult, error) {
	var out MongoFindResult
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/mongo/find", req, &out)
	return out, err
}
