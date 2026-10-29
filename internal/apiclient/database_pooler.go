package apiclient

import (
	"context"
	"net/http"
)

// DatabasePoolerResource is the connection pooler state of a managed
// Postgres database. URLs carry a password placeholder.
type DatabasePoolerResource struct {
	Enabled         bool   `json:"enabled"`
	PoolSize        int    `json:"pool_size,omitempty"`
	MaxClientConn   int    `json:"max_client_conn,omitempty"`
	Supported       bool   `json:"supported"`
	ImageConfigured bool   `json:"image_configured"`
	Container       string `json:"container,omitempty"`
	PooledURL       string `json:"pooled_url,omitempty"`
	DirectURL       string `json:"direct_url,omitempty"`
	Status          string `json:"status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Message         string `json:"message,omitempty"`
}

// GetDatabasePooler calls GET /api/v1/databases/{name}/pooler.
func (c *Client) GetDatabasePooler(ctx context.Context, name string) (DatabasePoolerResource, error) {
	var out DatabasePoolerResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/pooler", nil, &out)
	return out, err
}

// SetDatabasePooler calls POST (enable) or DELETE (disable) on .../pooler.
func (c *Client) SetDatabasePooler(ctx context.Context, name string, enabled bool) (DatabasePoolerResource, error) {
	method := http.MethodDelete
	if enabled {
		method = http.MethodPost
	}
	var out DatabasePoolerResource
	err := c.do(ctx, method, "/api/v1/databases/"+PathEscape(name)+"/pooler", nil, &out)
	return out, err
}

// DatabasePoolerTuning sizes the pooler; 0 keeps the control plane default.
type DatabasePoolerTuning struct {
	PoolSize      int `json:"pool_size,omitempty"`
	MaxClientConn int `json:"max_client_conn,omitempty"`
}

// EnableDatabasePooler calls POST .../pooler with optional sizing.
func (c *Client) EnableDatabasePooler(ctx context.Context, name string, tune DatabasePoolerTuning) (DatabasePoolerResource, error) {
	var out DatabasePoolerResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/pooler", tune, &out)
	return out, err
}
