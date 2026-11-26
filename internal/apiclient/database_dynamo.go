package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// DynamoTablesResult is the response of GET /databases/{name}/dynamo/tables.
type DynamoTablesResult struct {
	Tables    []json.RawMessage `json:"tables"`
	Names     []string          `json:"names"`
	Truncated bool              `json:"truncated"`
	Note      string            `json:"note"`
}

// DynamoReadRequest is the body of the dynamo query and scan routes. Values are DynamoDB JSON.
type DynamoReadRequest struct {
	Table    string          `json:"table"`
	Index    string          `json:"index,omitempty"`
	KeyName  string          `json:"key_name,omitempty"`
	KeyValue json.RawMessage `json:"key_value,omitempty"`
	Sort     json.RawMessage `json:"sort,omitempty"`
	Limit    int             `json:"limit,omitempty"`
	StartKey json.RawMessage `json:"exclusive_start_key,omitempty"`
	Confirm  bool            `json:"confirm,omitempty"`
}

// DynamoItemsResult is the response of the dynamo query and scan routes.
type DynamoItemsResult struct {
	Items            []json.RawMessage `json:"items"`
	Count            int               `json:"count"`
	ScannedCount     int               `json:"scanned_count"`
	LastKey          json.RawMessage   `json:"last_evaluated_key,omitempty"`
	ConsumedCapacity json.RawMessage   `json:"consumed_capacity,omitempty"`
	Truncated        bool              `json:"truncated"`
	Expensive        bool              `json:"expensive"`
	DurationMs       int64             `json:"duration_ms"`
}

// ListDynamoTables calls GET /api/v1/databases/{name}/dynamo/tables.
func (c *Client) ListDynamoTables(ctx context.Context, name, after string) (DynamoTablesResult, error) {
	path := "/api/v1/databases/" + PathEscape(name) + "/dynamo/tables"
	if after != "" {
		path += "?after=" + url.QueryEscape(after)
	}
	return getJSON[DynamoTablesResult](ctx, c, path)
}

// DynamoRead calls POST /api/v1/databases/{name}/dynamo/query or /scan.
func (c *Client) DynamoRead(ctx context.Context, name, op string, req DynamoReadRequest) (DynamoItemsResult, error) {
	var out DynamoItemsResult
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/dynamo/"+op, req, &out)
	return out, err
}
