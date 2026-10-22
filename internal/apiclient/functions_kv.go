package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// FunctionKVKey is one key in a function's store; Preview is truncated.
type FunctionKVKey struct {
	Key       string     `json:"key"`
	Size      int        `json:"size"`
	Preview   string     `json:"preview,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// FunctionKVList is the GET /api/v1/functions/{name}/kv body.
type FunctionKVList struct {
	Enabled bool            `json:"enabled"`
	Keys    []FunctionKVKey `json:"keys"`
	Count   int             `json:"count"`
	Bytes   int64           `json:"bytes"`
	Limits  struct {
		MaxKeyBytes   int   `json:"max_key_bytes"`
		MaxValueBytes int   `json:"max_value_bytes"`
		MaxKeys       int   `json:"max_keys"`
		MaxTotalBytes int64 `json:"max_total_bytes"`
		MaxTTLSeconds int   `json:"max_ttl_seconds"`
	} `json:"limits"`
}

// FunctionKVValue is one full value.
type FunctionKVValue struct {
	Key       string     `json:"key"`
	Value     string     `json:"value"`
	Size      int        `json:"size"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ListFunctionKV calls GET /api/v1/functions/{name}/kv.
func (c *Client) ListFunctionKV(ctx context.Context, name, prefix string, limit int) (FunctionKVList, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := functionPath(name, "kv")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out FunctionKVList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetFunctionKV calls GET /api/v1/functions/{name}/kv/value.
func (c *Client) GetFunctionKV(ctx context.Context, name, key string) (FunctionKVValue, error) {
	var out FunctionKVValue
	err := c.do(ctx, http.MethodGet, functionPath(name, "kv/value")+"?key="+url.QueryEscape(key), nil, &out)
	return out, err
}

// PutFunctionKV calls PUT /api/v1/functions/{name}/kv/value; a ttl of zero keeps the key.
func (c *Client) PutFunctionKV(ctx context.Context, name, key, value string, ttl time.Duration) error {
	body := map[string]any{"key": key, "value": value, "ttl_seconds": int(ttl / time.Second)}
	return c.do(ctx, http.MethodPut, functionPath(name, "kv/value"), body, nil)
}

// DeleteFunctionKV calls DELETE /api/v1/functions/{name}/kv/value.
func (c *Client) DeleteFunctionKV(ctx context.Context, name, key string) error {
	return c.do(ctx, http.MethodDelete, functionPath(name, "kv/value")+"?key="+url.QueryEscape(key), nil, nil)
}

// RotateFunctionKVToken calls POST /api/v1/functions/{name}/kv/token, enabling the store or replacing its token.
func (c *Client) RotateFunctionKVToken(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, functionPath(name, "kv/token"), nil, nil)
}
